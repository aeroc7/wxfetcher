package main

import (
	"encoding/json"
	"log"
	"math"
	"net/http"
	"strconv"
	"time"
	"wxdashboard/internal/db"
	"wxdashboard/internal/utils"
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
	LatestData WxProcessedRetrieval
}

type WxDbRead struct {
	RawJsonData []byte
}

const WXDB_FILENAME = "/home/ubuntu/wxdb.txt"

func (data *WxFetcher) fetchRemoteWxData() {
	get, err := http.Get("http://0.0.0.0:8433/stream")
	if err != nil {
		log.Fatal(err)
	}

	defer get.Body.Close()
	dec := json.NewDecoder(get.Body)

	for dec.More() {
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

		data, err := json.Marshal(processedData)
		if err != nil {
			log.Fatal(err)
		}

		db.WriteData(WXDB_FILENAME, string(data))
	}
}

func main() {
	var wxFetch WxFetcher

	httpHdlr := http.NewServeMux()
	httpHdlr.HandleFunc("GET /", httpHandler(wxFetch.healthCheck))
	httpHdlr.HandleFunc("GET /latest", httpHandler(wxFetch.httpEntry))

	// start our fetcher for wx data from rtl_433
	go wxFetch.fetchRemoteWxData()

	httpServer := &http.Server{
		Addr:    ":8080",
		Handler: httpHdlr,
	}

	log.Printf("listening on 8080")
	err := httpServer.ListenAndServe()
	if err != nil {
		log.Fatalf("error with listening on http server: %s", err)
	}
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

	requestFromTime, err := strconv.Atoi(req.URL.Query().Get("from"))
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
	}

	dbReadFull := db.Read(WXDB_FILENAME)
	var wxDbData []WxProcessedRetrieval

	log.Printf("after db read")

	err = json.Unmarshal(dbReadFull, &wxDbData)
	if err != nil {
		log.Printf("failed to unmarshal db json data. %v", err)
		res.WriteHeader(http.StatusInternalServerError)
		return
	}

	var startIndex = 0
	var endIndex = len(wxDbData) - 1

	for i, elem := range wxDbData {
		if i%4 == 0 {
			continue
		}

		loc, _ := time.LoadLocation("America/Los_Angeles")
		time, err := time.ParseInLocation(time.DateTime, elem.ReceiveTime, loc)
		if err != nil {
			log.Printf("failed to parse time entry at +%v %v", i, err)
			res.WriteHeader(http.StatusInternalServerError)
			return
		}

		if math.Abs(float64(requestFromTime)-float64(time.UTC().Unix())) < 12*1.5 {
			startIndex = i
		}

		if math.Abs(float64(requestToTime)-float64(time.UTC().Unix())) < 12*1.5 {
			endIndex = i
		}
	}

	jsonStr, err := json.Marshal(wxDbData[startIndex:endIndex])
	if err != nil {
		log.Printf("failed to marshal json data. %v", err)
		res.WriteHeader(http.StatusInternalServerError)
		return
	}

	res.Write([]byte(jsonStr))
}

func httpHandler(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}
