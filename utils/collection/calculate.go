package collection

type Number interface {
	int | int32 | int64 | float32 | float64
}

func SumBy[T any, N Number](s []T, valueSelector func(T) N) N {
	var result N
	for _, item := range s {
		value := valueSelector(item)
		result += value
	}
	return result
}

func SubtractBy[T any, N Number](s []T, valueSelector func(T) N) N {
	var result N
	for _, item := range s {
		value := valueSelector(item)
		result -= value
	}
	return result
}

func MultiplyBy[T any, N Number](s []T, valueSelector func(T) N) N {
	var result N
	for _, item := range s {
		value := valueSelector(item)
		result *= value
	}
	return result
}
