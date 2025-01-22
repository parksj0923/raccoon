package pointer

func NotNull[T any](source *T, defaultValue T) T {
	if source != nil {
		return *source
	}
	return defaultValue
}

func NotNullWithReturn[T any](source *T, defaultValue T, returnValue T) T {
	if source != nil {
		return returnValue
	}
	return defaultValue
}

func Create[T any](source T) *T {
	returnValue := new(T)
	returnValue = &source
	return returnValue
}
