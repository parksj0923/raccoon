package collection

import "sort"

func Sort[T any](s []T, less func(T, T) bool) {
	sort.Slice(s, func(i, j int) bool {
		return less(s[i], s[j])
	})
}

func GetOrDefaultByMap[K comparable, T any](source map[K]T, key K, defaultValue T) T {
	result, hit := source[key]
	if !hit {
		result = defaultValue
	}
	return result
}

func Filter[T any](s []T, predicate func(T) bool) []T {
	result := make([]T, 0, len(s))
	for _, v := range s {
		if predicate(v) {
			result = append(result, v)
		}
	}
	return result
}

func FilterMap[K comparable, T any](source map[K]T, predicate func(T) bool) map[K]T {
	result := make(map[K]T)
	for k, v := range source {
		if predicate(v) {
			result[k] = v
		}
	}
	return result
}

func FilterMapWithKey[K comparable, T any](source map[K]T, predicate func(K) bool) map[K]T {
	result := make(map[K]T)
	for k, v := range source {
		if predicate(k) {
			result[k] = v
		}
	}
	return result
}

func FirstOrNil[T any](s []T, predicate func(T) bool) *T {
	for _, v := range s {
		if predicate(v) {
			return &v
		}
	}
	return nil
}

func CountElements[T comparable](slice []T) map[T]int {
	count := make(map[T]int)
	for _, elem := range slice {
		count[elem]++
	}
	return count
}

func SameAllElements[T comparable](slice1, slice2 []T) bool {
	if len(slice1) != len(slice2) {
		return false
	}

	count1 := CountElements(slice1)
	count2 := CountElements(slice2)

	for key, value := range count1 {
		if count2[key] != value {
			return false
		}
	}
	return true
}

func SameAllElementsWithEqualFunc[T any](slice1, slice2 []T, equalFunc func(T, T) bool) bool {
	if len(slice1) != len(slice2) {
		return false
	}

	used := make([]bool, len(slice2))
	for _, item1 := range slice1 {
		found := false
		for j, item2 := range slice2 {
			if !used[j] && equalFunc(item1, item2) {
				used[j] = true
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func FoldLeft[S ~[]T, T any, B any](s S, z B, op func(B, T) B) B {
	result := z
	for _, v := range s {
		result = op(result, v)
	}
	return result
}

func MapFoldLeft(m map[string]int, z int, op func(int, int) int) int {
	result := z
	for _, v := range m {
		result = op(result, v)
	}
	return result
}

func Last[T any](s []T) (T, bool) {
	if len(s) == 0 {
		var zeroValue T
		return zeroValue, false
	}
	return s[len(s)-1], true
}

func FindElementsNotInFirst[T comparable](source, dest []T) []T {
	elementMap := make(map[T]bool)
	for _, item := range source {
		elementMap[item] = true
	}

	var notInFirst []T
	for _, item := range dest {
		if !elementMap[item] {
			notInFirst = append(notInFirst, item)
		}
	}

	return notInFirst
}

func Contains[T comparable](slice []T, item T) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}
	return false
}

func FilterByKey[K comparable, T any](keys []K, items []T, keySelector func(T) K) []T {
	keyMap := make(map[K]struct{}, len(keys))
	for _, key := range keys {
		keyMap[key] = struct{}{}
	}

	var result []T
	for _, item := range items {
		if _, exists := keyMap[keySelector(item)]; exists {
			result = append(result, item)
		}
	}
	return result
}
