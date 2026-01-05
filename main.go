package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/faiface/beep"
	"github.com/faiface/beep/mp3"
	"github.com/faiface/beep/speaker"
	"github.com/spf13/pflag"
	"github.com/surfaceyu/edge-tts-go/edgeTTS"
	"gopkg.in/yaml.v3"
)

const (
	appName = "readaloud"
)

type Config struct {
	Voice  string `yaml:"voice"`
	Rate   string `yaml:"rate"`
	Volume string `yaml:"volume"`
}

func synthesizeWithRetry(args edgeTTS.Args, maxRetries int, timeout time.Duration) error {
	var lastErr error
	for i := 0; i <= maxRetries; i++ {
		done := make(chan struct{})
		errChan := make(chan error, 1)

		go func() {
			defer func() {
				if r := recover(); r != nil {
					errChan <- fmt.Errorf("panic during TTS synthesis: %v", r)
				}
			}()
			tts := edgeTTS.NewTTS(args)
			if tts == nil {
				errChan <- fmt.Errorf("failed to create TTS object for media file: %s", args.WriteMedia)
				return
			}
			tts.AddText(args.Text, args.Voice, args.Rate, args.Volume)
			tts.Speak()
			close(done)
		}()

		select {
		case <-done:
			if i > 0 {
				log.Printf("TTS synthesis succeeded after %d retries.", i)
			}
			return nil // Success
		case err := <-errChan:
			lastErr = err
			log.Printf("Warning: TTS synthesis failed: %v. Retrying (%d/%d)...", err, i+1, maxRetries)
		case <-time.After(timeout):
			lastErr = fmt.Errorf("TTS synthesis timed out after %v", timeout)
			log.Printf("Warning: %v. Retrying (%d/%d)...", lastErr, i+1, maxRetries)
		}
	}
	return fmt.Errorf("TTS synthesis failed after %d retries: %w", maxRetries, lastErr)
}

func main() {
	var (
		voiceFlag      string
		rateFlag       string
		volumeFlag     string
		textFilePath   string
		writeMediaFile string
		saveDir        string
		name           string
	)

	pflag.StringVarP(&voiceFlag, "voice", "v", "", "Voice for TTS (e.g., en-US-AriaNeural)")
	pflag.StringVarP(&rateFlag, "rate", "r", "", "Set TTS rate (e.g., -10%)")
	pflag.StringVarP(&volumeFlag, "volume", "u", "", "Set TTS volume (e.g., 0%)")
	pflag.StringVarP(&textFilePath, "file", "f", "", "Path to a text file for TTS input")
	pflag.StringVarP(&writeMediaFile, "write-media", "w", "", "Write media output to file instead of playing (MP3)")
	pflag.StringVarP(&saveDir, "save-dir", "s", "", "Directory to save output files")
	pflag.StringVarP(&name, "name", "n", "", "Sub-folder name for sequential saving (requires -s)")

	pflag.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage of %s:
  %s [options] [text]
  %s --file <path> [options]
  echo "Hello world" | %s [options]

Options:
`, appName, appName, appName, appName)
		pflag.PrintDefaults()
	}

	pflag.Parse()

	if name != "" && saveDir == "" {
		log.Fatalf("Error: --name flag requires --save-dir flag to be set.")
	}

	// Load config from file
	config, err := loadConfig()
	if err != nil {
		log.Printf("Warning: Could not load config file: %v\n", err)
		config = &Config{} // Use empty config if loading fails
	}

	// Apply command-line overrides
	if voiceFlag != "" {
		config.Voice = voiceFlag
	}
	if rateFlag != "" {
		config.Rate = rateFlag
	}
	if volumeFlag != "" {
		config.Volume = volumeFlag
	}

	// Determine input text
	var inputText string
	stat, _ := os.Stdin.Stat()
	isStdin := (stat.Mode() & os.ModeCharDevice) == 0

	if textFilePath != "" && isStdin {
		pflag.Usage()
		log.Fatalf("Error: Cannot use both --file and stdin for input.")
	} else if textFilePath != "" {
		data, err := ioutil.ReadFile(textFilePath)
		if err != nil {
			log.Fatalf("Error reading file %s: %v", textFilePath, err)
		}
		inputText = string(data)
	} else if isStdin {
		reader := bufio.NewReader(os.Stdin)
		inputBytes, err := ioutil.ReadAll(reader)
		if err != nil {
			log.Fatalf("Error reading from stdin: %v", err)
		}
		inputText = string(inputBytes)
	} else if pflag.NArg() > 0 {
		inputText = strings.Join(pflag.Args(), " ")
	} else {
		pflag.Usage()
		log.Fatalf("Error: No input provided. Use --file, pipe to stdin, or provide text as arguments.")
	}

	if inputText == "" {
		pflag.Usage()
		log.Fatalf("Error: Input text is empty.")
	}

	const maxRetries = 10
	const timeout = 10 * time.Second

	// If writing to a file, use the old method. Otherwise, use the new streaming method.
	if writeMediaFile != "" {
		// Prepare TTS arguments
		args := edgeTTS.Args{
			Text:       inputText,
			Voice:      config.Voice,
			Rate:       config.Rate,
			Volume:     config.Volume,
			WriteMedia: writeMediaFile,
		}
		// Generate audio
		if err := synthesizeWithRetry(args, maxRetries, timeout); err != nil {
			log.Fatalf("Error synthesizing audio to %s: %v", writeMediaFile, err)
		}
	} else if saveDir != "" {
		// Create the save directory if it doesn't exist
		outputDir := saveDir
		if name != "" {
			outputDir = filepath.Join(saveDir, name)
		}
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			log.Fatalf("Error creating output directory %s: %v", outputDir, err)
		}

		const chunkSize = 100 // words
		const requestsPerMinute = 20
		const delay = time.Second * 60 / requestsPerMinute
		chunks := chunkTextByWords(inputText, chunkSize)
		totalChunks := len(chunks)
		padding := len(fmt.Sprintf("%d", totalChunks))

		for i, chunk := range chunks {
			outputFileName := fmt.Sprintf("%0*d.mp3", padding, i+1)
			outputFilePath := filepath.Join(outputDir, outputFileName)

			log.Printf("Synthesizing chunk (%d/%d) to %s...", i+1, totalChunks, outputFilePath)

			args := edgeTTS.Args{
				Text:       chunk,
				Voice:      config.Voice,
				Rate:       config.Rate,
				Volume:     config.Volume,
				WriteMedia: outputFilePath,
			}
			if err := synthesizeWithRetry(args, maxRetries, timeout); err != nil {
				log.Printf("Error synthesizing chunk %d to %s: %v. Skipping.", i+1, outputFilePath, err)
				continue
			}
			log.Printf("Chunk %d saved.", i+1)
			if i < totalChunks-1 {
				log.Printf("Rate limiting: waiting for %v before next request", delay)
				time.Sleep(delay)
			}
		}
		log.Printf("All chunks saved to %s", outputDir)
	} else {
		// Streaming playback
		const chunkSize = 100 // words
		const requestsPerMinute = 8
		const delay = time.Second * 60 / requestsPerMinute
		chunks := chunkTextByWords(inputText, chunkSize)
		totalChunks := len(chunks)
		audioQueue := make(chan string, 1) // Channel to hold the file path of the next audio chunk

		// Producer goroutine: fetches audio chunks
		go func() {
			for i, chunk := range chunks {
				log.Printf("Sending chunk (%d/%d) for synthesis...", i+1, totalChunks)
				tempFile, err := ioutil.TempFile("", fmt.Sprintf("readaloud-chunk-%d-*.mp3", i))
				if err != nil {
					log.Fatalf("Error creating temporary file: %v", err)
				}
				outputFilePath := tempFile.Name()
				tempFile.Close() // Close the file so the TTS library can write to it

				args := edgeTTS.Args{
					Text:       chunk,
					Voice:      config.Voice,
					Rate:       config.Rate,
					Volume:     config.Volume,
					WriteMedia: outputFilePath,
				}
				if err := synthesizeWithRetry(args, maxRetries, timeout); err != nil {
					log.Printf("Error synthesizing chunk %d: %v. Skipping.", i+1, err)
					os.Remove(outputFilePath) // Clean up failed temp file
					continue
				}
				log.Printf("Chunk %d received.", i+1)
				audioQueue <- outputFilePath
				if i < totalChunks-1 {
					log.Printf("Rate limiting: waiting for %v before next request", delay)
					time.Sleep(delay)
				}
			}
			close(audioQueue)
		}()

		// Consumer: plays audio chunks from the queue
		for audioFile := range audioQueue {
			log.Printf("Playing audio from %s", filepath.Base(audioFile))
			playAudio(audioFile)
			os.Remove(audioFile) // Clean up the chunk file after playing
		}
	}
}

func chunkTextByWords(text string, chunkSize int) []string {
	const wordTolerance = 10            // How many words +/- to look for a natural break.
	const newlineMarker = "||NEWLINE||" // A unique marker for newlines.

	// Define the punctuation to look for, in order of priority.
	sentenceEnders := []string{".", "!", "?"}
	naturalBreaks := []string{";", ":"}

	// Pre-process text to treat newlines as separate tokens.
	processedText := strings.ReplaceAll(text, "\r\n", " "+newlineMarker+" ")
	processedText = strings.ReplaceAll(processedText, "\n", " "+newlineMarker+" ")

	tokens := strings.Fields(processedText)
	if len(tokens) == 0 {
		return nil
	}

	var chunks []string
	start := 0
	for start < len(tokens) {
		targetEnd := start + chunkSize
		if targetEnd >= len(tokens) {
			// This is the last chunk.
			chunkTokens := tokens[start:]
			joined := strings.Join(chunkTokens, " ")
			finalChunk := strings.ReplaceAll(joined, newlineMarker, "\n")
			chunks = append(chunks, finalChunk)
			break
		}

		// Default split point is the target chunk size.
		splitPoint := targetEnd
		breakFound := false

		// Establish the search window.
		searchStart := targetEnd - wordTolerance
		if searchStart < start {
			searchStart = start
		}
		searchEnd := targetEnd + wordTolerance
		if searchEnd > len(tokens) {
			searchEnd = len(tokens)
		}

		// 1. Prioritize finding a sentence ender.
		for i := searchEnd - 1; i >= searchStart; i-- {
			for _, p := range sentenceEnders {
				if strings.HasSuffix(tokens[i], p) {
					splitPoint = i + 1 // Split after the word with the punctuation.
					breakFound = true
					break
				}
			}
			if breakFound {
				break
			}
		}

		// 2. If no sentence ender, find a natural break.
		if !breakFound {
			for i := searchEnd - 1; i >= searchStart; i-- {
				for _, p := range naturalBreaks {
					if strings.HasSuffix(tokens[i], p) {
						splitPoint = i + 1 // Split after the word with the punctuation.
						breakFound = true
						break
					}
				}
				if breakFound {
					break
				}
			}
		}

		// 3. If no natural break, find a newline.
		if !breakFound {
			for i := searchEnd - 1; i >= searchStart; i-- {
				if tokens[i] == newlineMarker {
					splitPoint = i // Split before the newline marker.
					breakFound = true
					break
				}
			}
		}

		// If our search results in a split point that doesn't advance our position,
		// we must force a split at the target length to prevent an infinite loop.
		if splitPoint <= start {
			splitPoint = targetEnd
		}

		chunkTokens := tokens[start:splitPoint]
		joined := strings.Join(chunkTokens, " ")
		// Restore newlines from the marker.
		finalChunk := strings.ReplaceAll(joined, newlineMarker, "\n")
		chunks = append(chunks, finalChunk)

		// Advance the start position for the next chunk.
		// If we split on a newline, the newline marker might be the next token, so we skip it.
		if splitPoint < len(tokens) && tokens[splitPoint] == newlineMarker {
			start = splitPoint + 1
		} else {
			start = splitPoint
		}
	}

	return chunks
}

func loadConfig() (*Config, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("could not get user home directory: %w", err)
	}
	configPath := filepath.Join(homeDir, ".config", appName, appName+".yaml")

	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("could not read config file %s: %w", configPath, err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("could not unmarshal config file %s: %w", configPath, err)
	}
	return &config, nil
}

func playAudio(filePath string) {
	f, err := os.Open(filePath)
	if err != nil {
		log.Printf("Error opening audio file: %v", err)
		return
	}
	defer f.Close()

	streamer, format, err := mp3.Decode(f)
	if err != nil {
		log.Printf("Error decoding MP3: %v", err)
		return
	}
	defer streamer.Close()

	speaker.Init(format.SampleRate, format.SampleRate.N(time.Second/10)) // Initialize speaker with correct sample rate

	var wg sync.WaitGroup
	wg.Add(1)
	speaker.Play(beep.Seq(streamer, beep.Callback(func() {
		wg.Done()
	})))

	wg.Wait()
}
