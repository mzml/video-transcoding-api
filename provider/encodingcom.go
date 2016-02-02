package provider

import (
	"errors"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/NYTimes/encoding-wrapper/encodingcom"
	"github.com/nytm/video-transcoding-api/config"
)

var errMissingData = InvalidConfigError("missing Encoding.com user id or key. Please define the environment variables ENCODINGCOM_USER_ID and ENCODINGCOM_USER_KEY or set these values in the configuration file")

type encodingComProvider struct {
	config *config.Config
	client *encodingcom.Client
}

func (e *encodingComProvider) TranscodeWithProfiles(sourceMedia string, profiles []Profile) (*JobStatus, error) {
	format := e.profilesToFormats(sourceMedia, profiles)
	resp, err := e.client.AddMedia([]string{sourceMedia}, format)
	if err != nil {
		return nil, err
	}
	return &JobStatus{ProviderJobID: resp.MediaID, StatusMessage: resp.Message}, nil
}

func (e *encodingComProvider) getResolution(output string, format encodingcom.Format) string {
	if output == "hls" || output == "thumb" {
		return ""
	}
	sizeSlice := strings.Split(format.Size, "x")
	if len(sizeSlice) > 1 {
		return sizeSlice[1] + "p"
	}
	return ""
}

func (e *encodingComProvider) getDestinations(sourceMedia string, format encodingcom.Format) []string {
	var destinations []string
	for _, output := range format.Output {
		extension := "." + output
		resolution := e.getResolution(output, format)

		sourceParts := strings.Split(sourceMedia, "/")
		sourceFilenamePart := sourceParts[len(sourceParts)-1]
		sourceFileName := strings.TrimSuffix(sourceFilenamePart, filepath.Ext(sourceFilenamePart))

		outputDestination := strings.TrimRight(e.config.EncodingCom.Destination, "/") + "/"
		finalDestination := outputDestination + sourceFileName + "_" + resolution + extension
		if output == "hls" {
			finalDestination = outputDestination + sourceFileName + "_hls/video.m3u8"
		}
		destinations = append(destinations, finalDestination)
	}
	return destinations
}

func (e *encodingComProvider) mapOutputs(outputs []string) []string {
	outputMap := map[string]string{
		"hls":   "advanced_hls",
		"thumb": "thumbnail",
	}
	for i, o := range outputs {
		if output, ok := outputMap[o]; ok {
			outputs[i] = output
		}
	}
	return outputs
}

func (e *encodingComProvider) profilesToFormats(sourceMedia string, profiles []Profile) []encodingcom.Format {
	var formats []encodingcom.Format
	for _, profile := range profiles {
		format := encodingcom.Format{
			Output:              profile.Output,
			Size:                profile.Size.String(),
			AudioCodec:          profile.AudioCodec,
			AudioBitrate:        profile.AudioBitRate,
			AudioChannelsNumber: profile.AudioChannelsNumber,
			AudioSampleRate:     profile.AudioSampleRate,
			Bitrate:             profile.BitRate,
			Framerate:           profile.FrameRate,
			KeepAspectRatio:     encodingcom.YesNoBoolean(profile.KeepAspectRatio),
			VideoCodec:          profile.VideoCodec,
			Keyframe:            []string{profile.KeyFrame},
			AudioVolume:         profile.AudioVolume,
			TwoPass:             encodingcom.YesNoBoolean(profile.TwoPassEncoding),
		}
		if profile.Rotate.set {
			format.Rotate = strconv.FormatUint(uint64(profile.Rotate.value), 10)
		} else {
			format.Rotate = "def"
		}
		format.Destination = e.getDestinations(sourceMedia, format)
		format.Output = e.mapOutputs(format.Output)
		formats = append(formats, format)
	}
	return formats
}

func (e *encodingComProvider) JobStatus(id string) (*JobStatus, error) {
	resp, err := e.client.GetStatus([]string{id})
	if err != nil {
		return nil, err
	}
	if len(resp) < 1 {
		return nil, errors.New("invalid value returned by the Encoding.com API: []")
	}
	return &JobStatus{
		ProviderJobID: id,
		ProviderName:  "encoding.com",
		Status:        e.statusMap(resp[0].MediaStatus),
		ProviderStatus: map[string]interface{}{
			"progress":          resp[0].Progress,
			"sourcefile":        resp[0].SourceFile,
			"timeleft":          resp[0].TimeLeft,
			"created":           resp[0].CreateDate,
			"started":           resp[0].StartDate,
			"finished":          resp[0].FinishDate,
			"destinationStatus": resp[0].Formats[0].Destinations,
		},
	}, nil
}

func (e *encodingComProvider) statusMap(encodingComStatus string) status {
	switch strings.ToLower(encodingComStatus) {
	case "new":
		return StatusQueued
	case "downloading", "ready to process", "waiting for encoder", "processing", "saving":
		return StatusStarted
	case "finished":
		return StatusFinished
	default:
		return StatusFailed
	}
}

// EncodingComProvider is the factory function for the Encoding.com provider.
func EncodingComProvider(cfg *config.Config) (TranscodingProvider, error) {
	if cfg.EncodingCom.UserID == "" || cfg.EncodingCom.UserKey == "" {
		return nil, errMissingData
	}
	client, err := encodingcom.NewClient("https://manage.encoding.com", cfg.EncodingCom.UserID, cfg.EncodingCom.UserKey)
	if err != nil {
		return nil, err
	}
	return &encodingComProvider{client: client, config: cfg}, nil
}
