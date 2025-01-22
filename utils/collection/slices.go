package collection

import "slices"

func ContainsInSlice[T comparable](checker []T, target []T) bool {
	for _, t := range target {
		if slices.Contains(checker, t) {
			return true
		}
	}
	return false
}
