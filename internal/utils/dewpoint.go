package utils

import "math"

func Dewpoint(T float32, RH float32) float32 {
	const a float32 = 17.625
	const b float32 = 243.04 // C

	// en.wikipedia.org/wiki/Dew_point#Calculating_the_dew_point
	var aTRH = (float32(math.Log(float64(RH/100.0))) + ((a * T) / (float32(b) + T)))
	dewpoint := (b * aTRH) / (a - aTRH)

	return dewpoint
}
