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
	"github.com/surfaceyu/edge-tts-go/edgeTTS"
	"github.com/spf13/pflag"
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

func main() {
	var (
		voiceFlag      string
		rateFlag       string
		volumeFlag     string
		textFilePath   string
		writeMediaFile string
	)

	pflag.StringVarP(&voiceFlag, "voice", "v", "", "Voice for TTS (e.g., en-US-AriaNeural)")
	pflag.StringVarP(&rateFlag, "rate", "r", "", "Set TTS rate (e.g., -10%)")
	pflag.StringVarP(&volumeFlag, "volume", "u", "", "Set TTS volume (e.g., 0%)")
	pflag.StringVarP(&textFilePath, "file", "f", "", "Path to a text file for TTS input")
	pflag.StringVarP(&writeMediaFile, "write-media", "w", "", "Write media output to file instead of playing (MP3)")

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
		tts := edgeTTS.NewTTS(args)
		tts.AddText(args.Text, args.Voice, args.Rate, args.Volume)
		tts.Speak()
	} else {
		// Streaming playback
		const chunkSize = 100 // words
		chunks := chunkTextByWords(inputText, chunkSize)
		audioQueue := make(chan string, 1) // Channel to hold the file path of the next audio chunk

		// Producer goroutine: fetches audio chunks
		go func() {
			for i, chunk := range chunks {
				log.Printf("Sending chunk %d for synthesis...", i+1)
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
				tts := edgeTTS.NewTTS(args)
				tts.AddText(args.Text, args.Voice, args.Rate, args.Volume)
				tts.Speak()
				log.Printf("Chunk %d received.", i+1)
				audioQueue <- outputFilePath
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
	const wordTolerance = 10        // How many words +/- to look for a natural break.
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