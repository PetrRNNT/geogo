package geocoder

type POI struct {
	ID      int64
	CityID  int64
	Name    string
	Address string
	Lat     float64
	Lon     float64
}
