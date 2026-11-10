// Package gradient merges adjacent quantised colour bands that together
// form one linear colour ramp, and fits each merged region a two-stop
// linear gradient in pixel coordinates. Colours are sRGB scaled to 0..1,
// the space an SVG gradient interpolates in. Float products are wrapped
// in float64() so arm64 and amd64 round alike.
package gradient
