package engine

import (
	"fmt"
	"os"
)

type chunk struct {
	offset int64
	length int
	value  string
}

func (e *Engine) splitIntoChunks(value string, chunkSize, overlap int) []chunk {
	var chunks []chunk
	for start := 0; start < len(value); start += chunkSize - overlap {
		end := start + chunkSize
		if end > len(value) {
			end = len(value)
		}
		chunks = append(chunks, chunk{offset: int64(start), length: end - start, value: value[start:end]})
		if end == len(value) {
			break
		}
	}
	return chunks
}

func (e *Engine) writeToTempFile(content string) (string, error) {
	tempFile, err := os.CreateTemp("", "chunk_*.txt")
	if err != nil {
		return "", err
	}
	defer tempFile.Close()

	_, err = tempFile.WriteString(content)
	if err != nil {
		return "", err
	}

	return tempFile.Name(), nil
}

func (e *Engine) getChunkFileName(docID, fieldName string, index int) string {
	chunkID := fmt.Sprintf("%s:%s:chunk-%d", docID, fieldName, index)
	return chunkID
}
