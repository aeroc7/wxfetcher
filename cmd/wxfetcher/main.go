package main

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"time"
	"wxdashboard/internal/db"
	"wxdashboard/internal/utils"

	"github.com/InfluxCommunity/influxdb3-go/v2/influxdb3"
)

type WxBrunner7in1Retrieval struct {
	ReceiveTime string  `json:"time"`
	ModelName   string  `json:"model"`
	Id          int     `json:"id"`
	Temperature float32 `json:"temperature_C"` // C
	Humidity    float32 `json:"humidity"`      // % / 100
	WindMax     float32 `json:"wind_max_m_s"`  // m/s
	WindAvg     float32 `json:"wind_avg_m_s"`  // m/s
	WindDir     int     `json:"wind_dir_deg"`  // degrees
	RainTotal   float32 `json:"rain_mm"`       // mm total
	LightLux    float32 `json:"light_lux"`     // lux
	UvIndex     float32 `json:"uvi"`           // uv index
	Battery     int     `json:"battery_ok"`    // bool
}

type WxBmp390Retrieval struct {
	ReceiveTime uint64  `json:"unix_time"`
	ModelName   string  `json:"model"`
	Temperature float32 `json:"temperature_C"`
	Pressure    float32 `json:"pressure_Pa"`
}

type WxScd30Retrieval struct {
	ReceiveTime      uint64  `json:"unix_time"`
	ModelName        string  `json:"model"`
	Temperature      float32 `json:"temperature_C"`
	Humidity         float32 `json:"humidity"`
	Co2Concentration float32 `json:"co2_concentration_ppm"`
}

type WxBrunner7in1ProcessedRetrieval struct {
	WxBrunner7in1Retrieval
	Dewpoint float32 `json:"dewpoint_C"` // C
}

type WxBmp390ProcessedRetrieval struct {
	WxBmp390Retrieval
}

type WxScd30ProcessedRetrieval struct {
	WxScd30Retrieval
}

type WxFetcher struct {
	DbClient *influxdb3.Client
}

type WxDbRead struct {
	RawJsonData []byte
}

func parseTimeInTimezone(timestr string, timezone string) (*time.Time, error) {
	loc, _ := time.LoadLocation(timezone)
	parsedTime, err := time.ParseInLocation(time.DateTime, timestr, loc)
	if err != nil {
		return nil, err
	}

	return &parsedTime, nil
}

func WxBrunner7in1ProcessedRetrieval_ToDatabase(processedData WxBrunner7in1ProcessedRetrieval) (point *influxdb3.Point, err error) {
	localTime, err := parseTimeInTimezone(processedData.ReceiveTime, "America/Los_Angeles")
	if err != nil {
		return nil, err
	}

	point = influxdb3.NewPointWithMeasurement(processedData.ModelName).
		SetTag("version", "wx1").
		SetField("id", processedData.Id).
		SetField("temperature_C", processedData.Temperature).
		SetField("dewpoint_C", processedData.Dewpoint).
		SetField("humidity", processedData.Humidity).
		SetField("wind_max_m_s", processedData.WindMax).
		SetField("wind_avg_m_s", processedData.WindAvg).
		SetField("wind_dir_deg", processedData.WindDir).
		SetField("rain_mm", processedData.RainTotal).
		SetField("light_lux", processedData.LightLux).
		SetField("uvi", processedData.UvIndex).
		SetTimestamp(localTime.UTC())

	return
}

func WxBmp390ProcessedRetrieval_ToDatabase(processedData WxBmp390ProcessedRetrieval) (point *influxdb3.Point, err error) {
	localTime := time.Unix(int64(processedData.ReceiveTime), 0)

	point = influxdb3.NewPointWithMeasurement(processedData.ModelName).
		SetTag("version", "wx1").
		SetField("temperature_C", processedData.Temperature).
		SetField("pressure_Pa", processedData.Pressure).
		SetTimestamp(localTime.UTC())

	return
}

func WxScd30ProcessedRetrieval_ToDatabase(processedData WxScd30ProcessedRetrieval) (point *influxdb3.Point, err error) {
	localTime := time.Unix(int64(processedData.ReceiveTime), 0)

	point = influxdb3.NewPointWithMeasurement(processedData.ModelName).
		SetTag("version", "wx1").
		SetField("temperature_C", processedData.Temperature).
		SetField("humidity", processedData.Humidity).
		SetField("co2_con_ppm", processedData.Co2Concentration).
		SetTimestamp(localTime.UTC())

	return
}

func (data *WxFetcher) fetchRemoteSdrWxData(ctx context.Context) {
	get, err := http.Get("http://0.0.0.0:8433/stream")
	if err != nil {
		log.Fatal(err)
	}

	defer get.Body.Close()
	dec := json.NewDecoder(get.Body)

	for dec.More() {
		var processedData WxBrunner7in1ProcessedRetrieval

		// get base brunner data.
		var dataInterface WxBrunner7in1Retrieval
		err := dec.Decode(&dataInterface)
		if err != nil {
			log.Printf("failed to decode data to WxBrunner7in1Retrieval, %s", err)
			continue
		}

		if dataInterface.ModelName != "Bresser-7in1" {
			continue
		}

		processedData.WxBrunner7in1Retrieval = dataInterface

		// calculate some extra points.
		processedData.Dewpoint = utils.Dewpoint(dataInterface.Temperature, dataInterface.Humidity)

		pointInDbForm, err := WxBrunner7in1ProcessedRetrieval_ToDatabase(processedData)
		if err != nil {
			log.Print(err)
			continue
		}

		pointAsPoints := []*influxdb3.Point{pointInDbForm}
		err = db.Write(data.DbClient, pointAsPoints)
		if err != nil {
			log.Print(err)
			continue
		}
	}
}

func (data *WxFetcher) setupWxDatabaseConn() (err error) {
	url := os.Getenv("INFLUX_HOST")
	token := os.Getenv("INFLUX_TOKEN")
	database := os.Getenv("INFLUX_DATABASE")

	data.DbClient, err = influxdb3.New(influxdb3.ClientConfig{
		Host:     url,
		Token:    token,
		Database: database,
	})

	defer func(client *influxdb3.Client) {
		err := client.Close()
		if err != nil {
			panic(err)
		}
		log.Printf("closing db conn")
	}(data.DbClient)

	if err != nil {
		return err
	}

	log.Printf("got db connection")
	return nil
}

func shutdownMessage() {
	NeedsExit := make(chan os.Signal, 1)

	signal.Notify(NeedsExit, os.Interrupt)
	signal.Notify(NeedsExit, syscall.SIGTERM)
	go func() {
		<-NeedsExit
		log.Printf("shutting down wxfetcher")
		os.Exit(0)
	}()
}

func main() {
	flag.Parse()

	var wxFetch WxFetcher

	shutdownMessage()
	err := wxFetch.setupWxDatabaseConn()
	if err != nil {
		log.Fatalf("database failed to initialize")
	}

	// start our fetcher for wx data from rtl_433
	ctx, _ := context.WithCancel(context.Background())
	go wxFetch.fetchRemoteSdrWxData(ctx)

	httpHdlr := http.NewServeMux()
	httpHdlr.HandleFunc("GET /", httpHandler(wxFetch.healthCheck))
	httpHdlr.HandleFunc("POST /loc1/BMP390", httpHandler(wxFetch.bmp390Entry))
	httpHdlr.HandleFunc("POST /loc1/SCD30", httpHandler(wxFetch.scd30Entry))

	httpServer := &http.Server{
		Addr:    ":8080",
		Handler: httpHdlr,
	}

	log.Printf("listening on 8080")
	err = httpServer.ListenAndServe()
	if err != nil {
		log.Fatalf("error with listening on http server: %s", err)
	}
}

func (data *WxFetcher) bmp390Entry(res http.ResponseWriter, req *http.Request) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(res, "failed to read body", http.StatusBadRequest)
		return
	}
	defer req.Body.Close()

	var processedData WxBmp390ProcessedRetrieval

	var dataInterface WxBmp390Retrieval
	err = json.Unmarshal(body, &dataInterface)
	if err != nil {
		log.Printf("failed to unmarshal data to WxBmp390Retrieval, %s", err)
		http.Error(res, "", http.StatusInternalServerError)
		return
	}

	if dataInterface.ModelName != "BMP390" {
		log.Printf("unknown data interface is %v", dataInterface.ModelName)
		http.Error(res, "", http.StatusInternalServerError)
		return
	}

	processedData.WxBmp390Retrieval = dataInterface

	pointInDbForm, err := WxBmp390ProcessedRetrieval_ToDatabase(processedData)
	if err != nil {
		log.Print(err)
		http.Error(res, "", http.StatusInternalServerError)
		return
	}

	pointAsPoints := []*influxdb3.Point{pointInDbForm}
	err = db.Write(data.DbClient, pointAsPoints)
	if err != nil {
		log.Print(err)
		http.Error(res, "", http.StatusInternalServerError)
		return
	}
}

func (data *WxFetcher) scd30Entry(res http.ResponseWriter, req *http.Request) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(res, "failed to read body", http.StatusBadRequest)
		return
	}
	defer req.Body.Close()
	io.Copy(io.Discard, req.Body)

	log.Printf("got scd30 entry")

	var processedData WxScd30ProcessedRetrieval

	var dataInterface WxScd30Retrieval
	err = json.Unmarshal(body, &dataInterface)
	if err != nil {
		log.Printf("failed to unmarshal data to WxScd30Retrieval, %s", err)
		http.Error(res, "", http.StatusInternalServerError)
		return
	}

	if dataInterface.ModelName != "SCD30" {
		log.Printf("unknown data interface is %v", dataInterface.ModelName)
		http.Error(res, "", http.StatusInternalServerError)
		return
	}

	processedData.WxScd30Retrieval = dataInterface

	pointInDbForm, err := WxScd30ProcessedRetrieval_ToDatabase(processedData)
	if err != nil {
		log.Print(err)
		http.Error(res, "", http.StatusInternalServerError)
		return
	}

	pointAsPoints := []*influxdb3.Point{pointInDbForm}
	err = db.Write(data.DbClient, pointAsPoints)
	if err != nil {
		log.Print(err)
		http.Error(res, "", http.StatusInternalServerError)
		return
	}

	res.WriteHeader(200)
	res.Write([]byte("OK"))
}

func (data *WxFetcher) healthCheck(res http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.Error(res, "", http.StatusMethodNotAllowed)
		return
	}
}

func httpHandler(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}
