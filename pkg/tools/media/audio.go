package media

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

type AudioTranscriber struct {
	modelPath    string
	language     string
	dockerClient *client.Client
	containerImg string
	initialized  bool
}

func NewAudioTranscriber(modelPath string) *AudioTranscriber {
	return &AudioTranscriber{
		modelPath:    modelPath,
		language:     "en",
		containerImg: "ghcr.io/ggerganov/whisper.cpp:main",
	}
}

func NewAudioTranscriberWithImage(modelPath, containerImage string) *AudioTranscriber {
	return &AudioTranscriber{
		modelPath:    modelPath,
		language:     "en",
		containerImg: containerImage,
	}
}

func (t *AudioTranscriber) SetLanguage(lang string) {
	t.language = lang
}

func (t *AudioTranscriber) Init() error {
	if t.initialized {
		return nil
	}

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("create docker client: %w", err)
	}

	t.dockerClient = cli
	t.initialized = true
	return nil
}

func (t *AudioTranscriber) Transcribe(data []byte) (*TranscriptionResult, error) {
	if err := t.Init(); err != nil {
		return nil, err
	}

	tmpDir, err := os.MkdirTemp("", "whisper-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	audioFile := filepath.Join(tmpDir, "audio.wav")
	if err := os.WriteFile(audioFile, data, 0644); err != nil {
		return nil, fmt.Errorf("write audio file: %w", err)
	}

	return t.transcribeWithDocker(context.Background(), audioFile, tmpDir)
}

func (t *AudioTranscriber) TranscribeFile(filePath string) (*TranscriptionResult, error) {
	if err := t.Init(); err != nil {
		return nil, err
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	if ext != ".wav" && ext != ".mp3" && ext != ".m4a" && ext != ".ogg" && ext != ".flac" {
		return nil, fmt.Errorf("unsupported audio format: %s", ext)
	}

	return t.transcribeWithDocker(context.Background(), filePath, filepath.Dir(filePath))
}

func (t *AudioTranscriber) transcribeWithDocker(ctx context.Context, audioPath, workDir string) (*TranscriptionResult, error) {
	if t.dockerClient == nil {
		return nil, fmt.Errorf("docker client not initialized")
	}

	absPath, err := filepath.Abs(audioPath)
	if err != nil {
		return nil, fmt.Errorf("get absolute path: %w", err)
	}

	containerName := fmt.Sprintf("whisper-%d", time.Now().UnixNano())

	containerConfig := &container.Config{
		Image: t.containerImg,
		Cmd: []string{
			"whisper-cli",
			"-m", "/models/base.en.bin",
			"-f", "/audio/" + filepath.Base(audioPath),
			"-l", t.language,
			"--output-txt",
			"--output-json",
		},
		WorkingDir: "/audio",
	}

	hostConfig := &container.HostConfig{
		Binds: []string{
			absPath + ":/audio/" + filepath.Base(audioPath),
		},
		AutoRemove: true,
		Resources: container.Resources{
			Memory: 2 * 1024 * 1024 * 1024,
		},
	}

	createResp, err := t.dockerClient.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, containerName)
	if err != nil {
		if isImageNotFound(err) {
			if err := t.pullImage(ctx); err != nil {
				return nil, fmt.Errorf("pull whisper image: %w", err)
			}
			createResp, err = t.dockerClient.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, containerName)
			if err != nil {
				return nil, fmt.Errorf("create container: %w", err)
			}
		} else {
			return nil, fmt.Errorf("create container: %w", err)
		}
	}

	err = t.dockerClient.ContainerStart(ctx, createResp.ID, container.StartOptions{})
	if err != nil {
		return nil, fmt.Errorf("start container: %w", err)
	}

	statusCh, errCh := t.dockerClient.ContainerWait(ctx, createResp.ID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if err != nil {
			return nil, fmt.Errorf("container wait: %w", err)
		}
	case <-statusCh:
	}

	out, err := t.dockerClient.ContainerLogs(ctx, createResp.ID, container.LogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		return nil, fmt.Errorf("get container logs: %w", err)
	}
	defer out.Close()

	var stdout, stderr bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdout, &stderr, out); err != nil {
		return nil, fmt.Errorf("read container output: %w", err)
	}

	result := t.parseWhisperOutput(stdout.String(), stderr.String())
	return result, nil
}

func (t *AudioTranscriber) pullImage(ctx context.Context) error {
	reader, err := t.dockerClient.ImagePull(ctx, t.containerImg, image.PullOptions{})
	if err != nil {
		return err
	}
	defer reader.Close()
	io.Copy(io.Discard, reader)
	return nil
}

func (t *AudioTranscriber) parseWhisperOutput(stdout, stderr string) *TranscriptionResult {
	result := &TranscriptionResult{
		Language: t.language,
		Text:     "",
	}

	lines := strings.Split(stdout, "\n")
	var textParts []string

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.Contains(line, "[") && strings.Contains(line, "]") {
			startIdx := strings.Index(line, "]")
			if startIdx != -1 && startIdx+1 < len(line) {
				text := strings.TrimSpace(line[startIdx+1:])
				if text != "" {
					textParts = append(textParts, text)
				}
			}
		}
	}

	result.Text = strings.Join(textParts, " ")

	if result.Text == "" {
		result.Text = t.extractTextFromRaw(stderr)
	}

	return result
}

func (t *AudioTranscriber) extractTextFromRaw(output string) string {
	var lines []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "whisper") && !strings.HasPrefix(line, "system_info") {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, " ")
}

func (t *AudioTranscriber) Close() {
	if t.dockerClient != nil {
		t.dockerClient.Close()
	}
}

type whisperJSONOutput struct {
	Transcription []struct {
		Timestamps struct {
			From string `json:"from"`
			To   string `json:"to"`
		} `json:"timestamps"`
		Offsets struct {
			From int `json:"from"`
			To   int `json:"to"`
		} `json:"offsets"`
		Text string `json:"text"`
	} `json:"transcription"`
}

func parseWhisperJSON(data []byte) (*TranscriptionResult, error) {
	var output whisperJSONOutput
	if err := json.Unmarshal(data, &output); err != nil {
		return nil, err
	}

	result := &TranscriptionResult{}
	var texts []string

	for _, seg := range output.Transcription {
		var start, end float64
		fmt.Sscanf(seg.Timestamps.From, "%f", &start)
		fmt.Sscanf(seg.Timestamps.To, "%f", &end)

		result.Segments = append(result.Segments, TranscriptionSegment{
			Start: start,
			End:   end,
			Text:  seg.Text,
		})
		texts = append(texts, seg.Text)
	}

	result.Text = strings.Join(texts, " ")
	return result, nil
}

func isImageNotFound(err error) bool {
	return strings.Contains(err.Error(), "No such image") ||
		strings.Contains(err.Error(), "not found")
}

func IsWhisperAvailable() bool {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return false
	}
	defer cli.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = cli.Ping(ctx)
	return err == nil
}

func GetModelPath() string {
	if path := os.Getenv("WHISPER_MODEL_PATH"); path != "" {
		return path
	}
	return ""
}

func IsDockerAvailable() bool {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return false
	}
	defer cli.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err = cli.Ping(ctx)
	return err == nil
}
