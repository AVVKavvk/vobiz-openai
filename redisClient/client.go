package redisClient

import (
	"github.com/AVVKavvk/openai-vobiz/models"
)

func AppendTranscript(transcript models.TranscriptModel, callId string) error {
	rc := GetRedisClient()
	key := "transcript:" + callId

	data, err := transcript.MarshalBinary()
	if err != nil {
		return err
	}
	return rc.RPush(key, data).Err()
}

func GetAllTranscript(callId string) []models.TranscriptModel {
	rc := GetRedisClient()
	key := "transcript:" + callId

	var transcript []models.TranscriptModel
	result := rc.LRange(key, 0, -1)

	for _, v := range result.Val() {
		var transcriptModel models.TranscriptModel
		err := transcriptModel.UnmarshalBinary([]byte(v))
		if err != nil {
			panic(err)
		}
		transcript = append(transcript, transcriptModel)
	}
	return transcript
}
