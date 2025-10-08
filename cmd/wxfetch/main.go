package main

import (
	"encoding/json"
	"log"
	"math"
	"net/http"
	"os"
	"strings"
	"sync"
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

type WxFetcher struct {
	LatestData WxProcessedRetrieval
	mu         sync.RWMutex
}

type WxDbRead struct {
	PointsRaw []string
}

const WXDB_FILENAME = "/home/ubuntu/wxdb.txt"

func readFromDb() (out *WxDbRead) {
	data, err := os.ReadFile(WXDB_FILENAME)
	if err != nil {
		log.Fatal(err)
	}

	var dbRead WxDbRead

	dbRead.PointsRaw = strings.Split(string(data), "\n")
	return &dbRead
}

func writeDataToDb(jsonStr string) {
	f, err := os.OpenFile(WXDB_FILENAME, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}

	_, err = f.WriteString(jsonStr + "\n")
	if err != nil {
		log.Fatal(err)
	}

	f.Sync()

	if err := f.Close(); err != nil {
		log.Fatal(err)
	}
}

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
		processedData.Dewpoint = dewPointCalculation(dataInterface)
		// processedData.HeatIndex = heatIndexCalculation(dataInterface)

		// store the newly received wx data into our data channel:
		data.mu.Lock()
		data.LatestData = processedData
		data.mu.Unlock()

		data, err := json.Marshal(processedData)
		if err != nil {
			log.Fatal(err)
		}

		writeDataToDb(string(data))
	}
}

func main() {
	var wxFetch WxFetcher

	// local api for graphing/visualization
	httpHdlr := http.NewServeMux()
	httpHdlr.HandleFunc("GET /latest", httpHandler(wxFetch.httpEntry))
	// httpHdlr.HandleFunc("GET /latest/dewpoint", httpHandler(wxFetch.dewPointCalculation))

	// start our fetcher for wx data from rtl_433
	go wxFetch.fetchRemoteWxData()

	httpServer := &http.Server{
		Addr:    ":8080",
		Handler: httpHdlr,
	}

	err := httpServer.ListenAndServe()
	if err != nil {
		log.Fatalf("error with listening on http server: %s", err)
	}
}

func heatIndexCalculation(latest WxBrunner7in1Retrieval) float32 {
	const c1 float64 = -8.78469475556
	const c2 float64 = 1.61139411
	const c3 float64 = 2.33854883889
	const c4 float64 = -0.14611605
	const c5 float64 = -0.012308094
	const c6 float64 = -0.0164248277778
	const c7 float64 = 0.002211732
	const c8 float64 = 0.00072546
	const c9 float64 = -0.000003582

	var T = float64(latest.Temperature)
	var R = float64(latest.Humidity)

	var heatIndex = c1 +
		(c2 * T) + (c3 * R) +
		(c4 * T * R) + (c5 * T * T) +
		(c6 * R * R) + (c7 * T * T * R) +
		(c8 * T * R * R) + (c9 * T * T * R * R)

	return float32(heatIndex)
}

func dewPointCalculation(latest WxBrunner7in1Retrieval) float32 {
	const a float32 = 17.625
	const b float32 = 243.04 // C

	// Magnus-Tetens formula
	var aTRH = (float32(math.Log(float64(latest.Humidity/100.0))) + ((a * latest.Temperature) / (float32(b) + latest.Temperature)))
	dewpoint := (b * aTRH) / (a - aTRH)

	return dewpoint
}

func (data *WxFetcher) httpEntry(res http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.Error(res, "", http.StatusMethodNotAllowed)
		return
	}

	dbRead := readFromDb()
	var jsonArrBuilder strings.Builder

	jsonArrBuilder.WriteString("[")
	for i, elem := range dbRead.PointsRaw {
		if i == (len(dbRead.PointsRaw) - 1) {
			break
		}

		if i != 0 {
			jsonArrBuilder.WriteString(",")
		}

		var unData WxFetcher
		err := json.Unmarshal([]byte(elem), &unData)
		if err != nil {
			log.Fatal(err)
		}

		jsonArrBuilder.WriteString(elem)
	}

	jsonArrBuilder.WriteString("]")

	res.Write([]byte(jsonArrBuilder.String()))
}

func httpHandler(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// username, password, ok := r.BasicAuth()

		next.ServeHTTP(w, r)
	})
}
