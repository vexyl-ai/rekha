// Package colour converts sRGB to the OKLab perceptual colour space, in
// which the quantiser measures colour differences.
package colour

import (
	"fmt"
	"math"
)

// OKLab is a colour in Ottosson's OKLab space: L is lightness in [0, 1],
// A and B are the opponent axes, roughly within ±0.4. Euclidean distance
// approximates perceived difference; white to black is 1.
type OKLab struct{ L, A, B float64 }

// srgbToLinear decodes one 8-bit sRGB channel to linear light.
func srgbToLinear(c uint8) float64 {
	v := float64(c) / 255
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}

// linearToSRGB8 encodes linear light to an 8-bit sRGB channel, clamped.
func linearToSRGB8(v float64) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 1 {
		return 255
	}
	if v <= 0.0031308 {
		v *= 12.92
	} else {
		v = 1.055*math.Pow(v, 1/2.4) - 0.055
	}
	return uint8(math.Round(v * 255))
}

// FromSRGB8 converts an 8-bit sRGB colour to OKLab.
func FromSRGB8(r, g, b uint8) OKLab {
	lr, lg, lb := srgbToLinear(r), srgbToLinear(g), srgbToLinear(b)
	l := math.Cbrt(0.4122214708*lr + 0.5363325363*lg + 0.0514459929*lb)
	m := math.Cbrt(0.2119034982*lr + 0.6806995451*lg + 0.1073969566*lb)
	s := math.Cbrt(0.0883024619*lr + 0.2817188376*lg + 0.6299787005*lb)
	return OKLab{
		L: 0.2104542553*l + 0.7936177850*m - 0.0040720468*s,
		A: 1.9779984951*l - 2.4285922050*m + 0.4505937099*s,
		B: 0.0259040371*l + 0.7827717662*m - 0.8086757660*s,
	}
}

// ToSRGB8 converts to 8-bit sRGB, clamping colours outside the gamut.
func (c OKLab) ToSRGB8() (r, g, b uint8) {
	l := c.L + 0.3963377774*c.A + 0.2158037573*c.B
	m := c.L - 0.1055613458*c.A - 0.0638541728*c.B
	s := c.L - 0.0894841775*c.A - 1.2914855480*c.B
	l, m, s = l*l*l, m*m*m, s*s*s
	lr := 4.0767416621*l - 3.3077115913*m + 0.2309699292*s
	lg := -1.2684380046*l + 2.6097574011*m - 0.3413193965*s
	lb := -0.0041960863*l - 0.7034186147*m + 1.7076147010*s
	return linearToSRGB8(lr), linearToSRGB8(lg), linearToSRGB8(lb)
}

// Dist is the Euclidean distance between two colours.
func Dist(a, b OKLab) float64 {
	dl, da, db := a.L-b.L, a.A-b.A, a.B-b.B
	return math.Sqrt(float64(dl*dl) + float64(da*da) + float64(db*db))
}

// Hex returns the colour as lowercase "#rrggbb".
func (c OKLab) Hex() string {
	r, g, b := c.ToSRGB8()
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}
