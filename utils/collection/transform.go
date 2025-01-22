package collection

import (
	"math"
)

func Map[T any, V any](sources []T, f func(T) V) []V {
	results := make([]V, len(sources))
	for i, v := range sources {
		results[i] = f(v)
	}
	return results
}

func MapNotNil[T any, V any](sources []T, f func(T) *V) []V {
	results := make([]V, 0, len(sources))
	for _, v := range sources {
		result := f(v)
		if result != nil {
			results = append(results, *result)
		}
	}
	return results
}

func FlatMap[T any, V any](sources []T, f func(T) []V) []V {
	results := make([]V, 0)
	for _, v := range sources {
		array := f(v)
		for _, k := range array {
			results = append(results, k)
		}
	}
	return results
}

func GroupBy[T any, V comparable](sources []T, f func(T) V) map[V][]T {
	var result = make(map[V][]T)
	for _, v := range sources {
		values, exist := result[f(v)]
		if exist {
			result[f(v)] = append(values, v)
		} else {
			add := make([]T, 1)
			add[0] = v
			result[f(v)] = add
		}
	}
	return result
}

func GroupByWithMapNotNil[T, M any, V comparable](sources []T, f func(T) V, m func(T) *M) map[V][]M {
	var result = make(map[V][]M)
	for _, v := range sources {
		values, exist := result[f(v)]
		conv := m(v)
		if conv != nil {
			if exist {
				result[f(v)] = append(values, *conv)
			} else {
				add := make([]M, 1)
				add[0] = *conv
				result[f(v)] = add
			}
		}
	}
	return result
}

func AssociateBy[T any, V comparable](sources []T, f func(T) V) map[V]T {
	var result = make(map[V]T)
	for _, v := range sources {
		result[f(v)] = v
	}
	return result
}

func AssociateByWithMapNotNil[T, M any, V comparable](sources []T, f func(T) V, m func(T) *M) map[V]M {
	var result = make(map[V]M)
	for _, v := range sources {
		conv := m(v)
		if conv != nil {
			result[f(v)] = *conv
		}
	}
	return result
}

func DistinctBy[T any, K comparable](s []T, keySelector func(T) K) []T {
	keys := make(map[K]bool)
	result := make([]T, 0)
	for _, item := range s {
		key := keySelector(item)
		if _, exists := keys[key]; !exists {
			keys[key] = true
			result = append(result, item)
		}
	}
	return result
}

func Partition[T any](s []T, predicate func(T) bool) ([]T, []T) {
	capacity := len(s)
	trueParts := make([]T, 0, capacity)
	falseParts := make([]T, 0, capacity)
	for _, item := range s {
		if predicate(item) {
			trueParts = append(trueParts, item)
		} else {
			falseParts = append(falseParts, item)
		}
	}
	return trueParts, falseParts
}

func Partitions[T any](s []T, predicates ...func(T) bool) [][]T {
	conditionCount := len(predicates)
	if conditionCount == 0 {
		return nil
	}

	partitions := make([][]T, conditionCount)

	for _, item := range s {
		for i, predicate := range predicates {
			if predicate(item) {
				partitions[i] = append(partitions[i], item)
			}
		}
	}
	return partitions
}

func SplitSliceIntoNSubSlices[T any](slice []T, n int) [][]T {
	if n <= 0 {
		return nil
	}

	totalLen := len(slice)
	if totalLen <= n {
		result := make([][]T, totalLen)
		for i, v := range slice {
			result[i] = []T{v}
		}
		return result
	}

	subSliceSize := int(math.Ceil(float64(totalLen) / float64(n)))
	var subSlices [][]T

	for i := 0; i < totalLen; i += subSliceSize {
		end := i + subSliceSize
		if end > totalLen {
			end = totalLen
		}
		subSlices = append(subSlices, slice[i:end])
	}

	return subSlices
}

func ChunkSlice[T any](slice []T, chunkSize int) [][]T {
	var chunks [][]T
	for chunkSize < len(slice) {
		slice, chunks = slice[chunkSize:], append(chunks, slice[0:chunkSize:chunkSize])
	}
	chunks = append(chunks, slice)
	return chunks
}

func New[T any](params ...T) []T {
	return params
}
