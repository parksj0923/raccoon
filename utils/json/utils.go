package json

import "encoding/json"

func SerializeMessageBodies[T any](messages []T) [][]byte {
	size := len(messages)
	results := make([][]byte, size)
	for index, it := range messages {
		bodyJsonBytes, err := json.Marshal(it)
		if err != nil {
			bodyJsonBytes = []byte((""))
		}
		results[index] = bodyJsonBytes
	}
	return results
}

func SerializeMessageBody[T any](message T) []byte {
	bodyJsonBytes, err := json.Marshal(message)
	if err != nil {
		bodyJsonBytes = []byte((""))
	}
	return bodyJsonBytes
}

func DeserializeMessageBodies[T any](messages [][]byte) []T {
	size := len(messages)
	results := make([]T, size)
	for index, it := range messages {
		err := json.Unmarshal(it, &results[index])
		if err != nil {
			results[index] = *new(T)
		}
	}
	return results
}

func DeserializeMessageBody[T any](message []byte) T {
	var result T
	err := json.Unmarshal(message, &result)
	if err != nil {
		result = *new(T)
	}
	return result
}
