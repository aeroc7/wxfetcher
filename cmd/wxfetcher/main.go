package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
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

type WxProcessedRetrieval struct {
	WxBrunner7in1Retrieval
	Dewpoint float32 `json:"dewpoint_C"` // C
	// HeatIndex float32 `json:"heatindex_C"` // C
}

type MetricType struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type MetricReturnType struct {
	Value    string       `json:"value"`
	Payloads []MetricType `json:"payloads"`
}

type MetricQueryValueType[T any] struct {
	Datapoints []T
}

type MetricQueryType[T any] struct {
	Name      string                  `json:"target"`
	Datapoint MetricQueryValueType[T] `json:"datapoints"`
}

type MetricQueryTypeArray struct {
	Metrics []interface{} `json:""`
}

type WxFetcher struct {
	LatestData   WxProcessedRetrieval
	LocLatitude  float32
	LocLongitude float32

	DbClient  *influxdb3.Client
	WaitExit sync.WaitGroup
}

type WxDbRead struct {
	RawJsonData []byte
}

func JsonToInfluxDbPoint(processedData WxProcessedRetrieval) (point *influxdb3.Point, err error) {
	loc, _ := time.LoadLocation("America/Los_Angeles")
	parsedTime, err := time.ParseInLocation(time.DateTime, processedData.ReceiveTime, loc)
	if err != nil {
		return nil, err
	}

	point = influxdb3.NewPointWithMeasurement(processedData.ModelName).
		SetTag("version", "wx1").
		SetField("id", processedData.Id).
		SetField("temperature_C", processedData.Temperature).
		SetField("humidity", processedData.Humidity).
		SetField("wind_max_m_s", processedData.WindMax).
		SetField("wind_avg_m_s", processedData.WindAvg).
		SetField("wind_dir_deg", processedData.WindDir).
		SetField("rain_mm", processedData.RainTotal).
		SetField("light_lux", processedData.LightLux).
		SetField("uvi", processedData.UvIndex).
		SetTimestamp(parsedTime.UTC())

	return
}

func (data *WxFetcher) fetchRemoteWxData() {
	get, err := http.Get("http://0.0.0.0:8433/stream")
	if err != nil {
		log.Fatal(err)
	}

	defer get.Body.Close()
	dec := json.NewDecoder(get.Body)

	for dec.More() {
		data.WaitExit.Add(1)
		defer data.WaitExit.Done()
		var processedData WxProcessedRetrieval

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
		// processedData.HeatIndex = heatIndexCalculation(dataInterface)

		pointInDbForm, err := JsonToInfluxDbPoint(processedData)
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

		log.Printf("wrote point to db successfully")

		time.Sleep(7 * time.Second)
	}
}

func (data *WxFetcher) setupWxDatabaseConn() {
	url := os.Getenv("INFLUX_HOST")
	token := os.Getenv("INFLUX_TOKEN")
	database := os.Getenv("INFLUX_DATABASE")

	client, err := influxdb3.New(influxdb3.ClientConfig{
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
	}(client)

	if err != nil {
		log.Print(err)
		return
	}

	data.DbClient = client
	log.Printf("got db connection")
}

func (data *WxFetcher) importJsonDbData(jsondb string) {
	dbReadFull := db.ReadOld(jsondb)
	log.Printf("read db at jsondb %v of size %v", jsondb, len(dbReadFull))

	var wxDbRead []WxProcessedRetrieval
	json.Unmarshal(dbReadFull, &wxDbRead)

	var pointsList []*influxdb3.Point = nil

	insertDb := func(client *influxdb3.Client, pointsList []*influxdb3.Point) {
		err := db.Write(client, pointsList)
		if err != nil {
			log.Fatalf("db write error %v", err)
		}
	}

	for i, elem := range wxDbRead {
		pointDbForm, err := JsonToInfluxDbPoint(elem)
		if err != nil {
			log.Fatalf("error %v", err)
		}

		pointsList = append(pointsList, pointDbForm)

		if i%25000 == 0 {
			insertDb(data.DbClient, pointsList)

			log.Printf("inserted %v", len(pointsList))
			pointsList = nil
		}
	}

	// finish pointsList < 100
	if pointsList != nil {
		insertDb(data.DbClient, pointsList)
		log.Printf("inserted %v", len(pointsList))
	}
}

func main() {
	importParse := flag.Bool("import", false, "import wxdb json data to database")
	jsonDbPath := flag.String("jsondb", "wxdb.txt", "json db path for importing")
	// latVal := flag.Float64("latitude", 42.3804, "location latitude")
	// lonVal := flag.Float64("longitude", -103.4369, "location longitude")
	flag.Parse()

	// err := wxgov.GetWideAreaWeatherParams(*latVal, *lonVal)
	// if err != nil {
	// 	log.Fatalf("got error with point metadata: %v", err)
	// }

	// return

	var wxFetch WxFetcher
	NeedsExit := make(chan os.Signal, 1)

	// support a graceful shutdown.
	signal.Notify(NeedsExit, os.Interrupt)
	signal.Notify(NeedsExit, syscall.SIGTERM)
	go func() {
		<-NeedsExit
		log.Printf("shutting down wxfetcher")
		wxFetch.WaitExit.Wait()
		os.Exit(0)
	}()

	wxFetch.setupWxDatabaseConn()

	if *importParse && *jsonDbPath != "" {
		log.Printf("importing json db data.")
		wxFetch.importJsonDbData(*jsonDbPath)
	}

	// start our fetcher for wx data from rtl_433
	go wxFetch.fetchRemoteWxData()

	// wxFetch.LocLatitude = float32(*latVal)
	// wxFetch.LocLongitude = float32(*lonVal)

	// httpHdlr := http.NewServeMux()
	// httpHdlr.HandleFunc("GET /", httpHandler(wxFetch.healthCheck))
	// httpHdlr.HandleFunc("GET /latest", httpHandler(wxFetch.httpEntry))

	// start our fetcher for wx data from rtl_433
	// go wxFetch.fetchRemoteWxData()

	// httpServer := &http.Server{
	// 	Addr:    ":8080",
	// 	Handler: httpHdlr,
	// }

	// log.Printf("listening on 8080")
	// err := httpServer.ListenAndServe()
	// if err != nil {
	// 	log.Fatalf("error with listening on http server: %s", err)
	// }*/
}

func (data *WxFetcher) healthCheck(res http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.Error(res, "", http.StatusMethodNotAllowed)
		return
	}
}

func (data *WxFetcher) httpEntry(res http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.Error(res, "", http.StatusMethodNotAllowed)
		return
	}

	/*requestFromTime, err := strconv.Atoi(req.URL.Query().Get("from"))
	if err != nil {
		log.Printf("from time error.")
		res.WriteHeader(http.StatusInternalServerError)
		return
	}

	requestToTime, err := strconv.Atoi(req.URL.Query().Get("to"))
	if err != nil {
		log.Printf("to time error.")
		res.WriteHeader(http.StatusInternalServerError)
		return
	}*/

	/*dbReadFull := db.Read(WXDB_FILENAME)

	var p fastjson.Parser
	pj, err := p.Parse(string(dbReadFull))
	if err != nil {
		log.Printf("failed to parse db json data. %v", err)
		res.WriteHeader(http.StatusInternalServerError)
		return
	}

	obj, err := pj.Array()
	if err != nil {
		log.Fatalf("cannot obtain object from json value: %s", err)
	}

	var startIndex = 0
	var endIndex = len(obj) - 1
	const INCREMENT = 3

	for i, elem := range obj {
		loc, _ := time.LoadLocation("America/Los_Angeles")
		parsedTime, err := time.ParseInLocation(time.DateTime, string(elem.GetStringBytes("time")), loc)
		if err != nil {
			log.Printf("failed to parse time entry at +%v %v", i, err)
			res.WriteHeader(http.StatusInternalServerError)
			return
		}

		// limit to 6 hours ago.
		timeLimitUnix := time.Now().Add(-time.Hour * time.Duration(3)).UTC().Unix()

		result := math.Abs(float64(timeLimitUnix) - float64(parsedTime.UTC().Unix()))
		// log.Printf("%v %v", string(elem.GetStringBytes("time")), result)

		if result < 36 {
			startIndex = i
		}

		/*log.Printf("from time: %v %v to: %v", requestFromTime, parsedTime.UTC().Unix(), requestToTime)
		if math.Abs(float64(requestFromTime)-float64(parsedTime.UTC().Unix())) < 36 {
			startIndex = i
		}

		if math.Abs(float64(requestToTime)-float64(parsedTime.UTC().Unix())) < 36 {
			endIndex = i
		}
	}

	log.Printf("startIndex %v endIndex %v", startIndex, endIndex)

	// crude json array format return.
	var jsonStr []byte
	jsonStr = []byte("[")

	for i := startIndex; i < endIndex; i += INCREMENT {
		jsonStr = obj[i].MarshalTo(jsonStr)
		if (i + INCREMENT) < endIndex {
			jsonStr = append(jsonStr, byte(','))
		}
	}

	jsonStr = append(jsonStr, byte(']'))

	res.Header().Add("Content-Type", "application/json")
	res.Write([]byte(jsonStr))*/
}

func httpHandler(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}
