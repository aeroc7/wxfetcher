package db

import (
	"context"
	"log"
	"os"
	"strings"

	"github.com/InfluxCommunity/influxdb3-go/v2/influxdb3"
	"github.com/influxdata/line-protocol/v2/lineprotocol"
)

func Write(client *influxdb3.Client, points []*influxdb3.Point) (err error) {
	err = client.WritePoints(context.Background(), points, influxdb3.WithPrecision(lineprotocol.Second))
	return
}

func ReadOld(filename string) (out []byte) {
	data, err := os.ReadFile(filename)
	if err != nil {
		log.Fatal(err)
	}

	dbLines := strings.Split(string(data), "\n")

	// format data to proper json spec.
	var jsonArrBuilder strings.Builder
	jsonArrBuilder.WriteString("[")
	for i, elem := range dbLines {
		if i == (len(dbLines) - 1) {
			break
		}

		if i != 0 {
			jsonArrBuilder.WriteString(",")
		}

		jsonArrBuilder.WriteString(elem)
	}

	jsonArrBuilder.WriteString("]")
	return []byte(jsonArrBuilder.String())
}

func WriteDataOld(filename string, jsonStr string) {
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
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
