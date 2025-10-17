package db

import (
	"log"
	"os"
	"strings"
)

func Read(filename string) (out []byte) {
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

func WriteData(filename string, jsonStr string) {
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
