package model

import (
	"strconv"
	"strings"
	"time"

	"golang.org/x/exp/constraints"
)

// Series is a time series of values
type Series[T constraints.Ordered] []T

// Values returns the values of the series
func (s Series[T]) Values() []T {
	return s
}

// Length returns the number of values in the series
func (s Series[T]) Length() int {
	return len(s)
}

// Last returns the last value of the series given a past index position
func (s Series[T]) Last(position int) T {
	return s[len(s)-1-position]
}

// LastValues returns the last values of the series given a size
func (s Series[T]) LastValues(size int) []T {
	if l := len(s); l > size {
		return s[l-size:]
	}
	return s
}

// Crossover returns true if the last value of the series is greater than the last value of the reference series
func (s Series[T]) Crossover(ref Series[T]) bool {
	return s.Last(0) > ref.Last(0) && s.Last(1) <= ref.Last(1)
}

// Crossunder returns true if the last value of the series is less than the last value of the reference series
func (s Series[T]) Crossunder(ref Series[T]) bool {
	return s.Last(0) <= ref.Last(0) && s.Last(1) > ref.Last(1)
}

// Cross returns true if the last value of the series is greater than the last value of the
// reference series or less than the last value of the reference series
func (s Series[T]) Cross(ref Series[T]) bool {
	return s.Crossover(ref) || s.Crossunder(ref)
}

// NumDecPlaces returns the number of decimal places of a float64
func NumDecPlaces(v float64) int64 {
	s := strconv.FormatFloat(v, 'f', -1, 64)
	i := strings.IndexByte(s, '.')
	if i > -1 {
		return int64(len(s) - i - 1)
	}
	return 0
}

type Dataframe struct {
	Pair string

	Close  Series[float64]
	Open   Series[float64]
	High   Series[float64]
	Low    Series[float64]
	Volume Series[float64]

	Time       []time.Time
	LastUpdate time.Time

	// Custom user metadata
	Metadata map[string]Series[float64]
}

func (df Dataframe) Sample(positions int) Dataframe {
	size := len(df.Time)
	start := size - positions
	if start <= 0 {
		return df
	}

	sample := Dataframe{
		Pair:       df.Pair,
		Close:      df.Close.LastValues(positions),
		Open:       df.Open.LastValues(positions),
		High:       df.High.LastValues(positions),
		Low:        df.Low.LastValues(positions),
		Volume:     df.Volume.LastValues(positions),
		Time:       df.Time[start:],
		LastUpdate: df.LastUpdate,
		Metadata:   make(map[string]Series[float64]),
	}

	for key := range df.Metadata {
		sample.Metadata[key] = df.Metadata[key].LastValues(positions)
	}

	return sample
}
