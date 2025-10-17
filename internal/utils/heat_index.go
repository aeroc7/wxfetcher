package utils

func HeatIndex(T float64, RH float64) float32 {
	const c1 float64 = -8.78469475556
	const c2 float64 = 1.61139411
	const c3 float64 = 2.33854883889
	const c4 float64 = -0.14611605
	const c5 float64 = -0.012308094
	const c6 float64 = -0.0164248277778
	const c7 float64 = 0.002211732
	const c8 float64 = 0.00072546
	const c9 float64 = -0.000003582

	// en.wikipedia.org/wiki/Heat_index#Formula
	var heatIndex = c1 +
		(c2 * T) + (c3 * RH) +
		(c4 * T * RH) + (c5 * T * T) +
		(c6 * RH * RH) + (c7 * T * T * RH) +
		(c8 * T * RH * RH) + (c9 * T * T * RH * RH)

	return float32(heatIndex)
}
