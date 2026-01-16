package redisClient

import (
	"encoding/json"

	"github.com/AVVKavvk/openai-vobiz/models"
)

func AppendTranscript(transcript models.TranscriptModel, callId string) error {
	rc := GetRedisClient()
	key := "transcript:" + callId

	return rc.RPush(key, transcript).Err()
}

func GetAllTranscript(callId string) []models.TranscriptModel {
	rc := GetRedisClient()
	key := "transcript:" + callId

	var transcript []models.TranscriptModel
	result := rc.LRange(key, 0, -1)

	for _, v := range result.Val() {
		var transcriptModel models.TranscriptModel
		err := json.Unmarshal([]byte(v), &transcriptModel)
		if err != nil {
			panic(err)
		}
		transcript = append(transcript, transcriptModel)
	}
	return transcript
}
