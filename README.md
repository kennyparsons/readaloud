# readaloud

`readaloud` is a command-line tool for text-to-speech synthesis using Microsoft Edge's online service. It supports streaming playback, saving to files, and splitting long texts into manageable audio chunks.

## Prerequisites

*   **Node.js**: This tool utilizes [Travis'](https://github.com/travisvn/universal-edge-tts) `universal-edge-tts` package, which is java/typescript. Therefore, it has an os requirement of `node` to be installed and available in your `$PATH`. As such, this project is more of a value-added wrapper around the core tts project, allowing for easy config of voice, rate, saving audio, and speeding up playback by chunking. 

## Installation

### Homebrew (macOS/Linux)

```bash
brew install kennyparsons/readaloud/readaloud
```

## Usage

### Streaming Playback
To play the synthesized speech directly (chunks are buffered and played sequentially):

```bash
# Read from arguments
readaloud "Hello, world! This is a test."

# Read from a file
readaloud --file my_book.txt

# Read from stdin (clipboard example)
pbpaste | readaloud
```

### Save to File
To write the output to a single media file:

```bash
readaloud "Hello, world!" --write-media hello.mp3
```

### Save to Directory (Chunked)
To split a long text into multiple audio files (100 words per chunk) and save them sequentially:

```bash
# Saves 01.mp3, 02.mp3, etc. to ./audiobook/
readaloud --file long_text.txt --save-dir ./audiobook

# Saves 01.mp3, 02.mp3 to ./library/chapter1/
readaloud --file chapter1.txt --save-dir ./library --name "chapter1"
```

## Configuration

You can configure default settings by creating a YAML file at `~/.config/readaloud/readaloud.yaml`:

```yaml
voice: en-US-AriaNeural
rate: +0%
volume: +0%
```

> For a list of available voices, please visit [tts.travisvn.com](https://tts.travisvn.com/)

### Flags

| Flag | Short | Description |
| :--- | :--- | :--- |
| `--voice` | `-v` | Voice for TTS (e.g., `en-US-AriaNeural`, `en-GB-SoniaNeural`). |
| `--rate` | `-r` | Set TTS rate (e.g., `-10%`, `+50%`). |
| `--volume` | `-u` | Set TTS volume (e.g., `-50%`). |
| `--file` | `-f` | Path to a text file for input. |
| `--write-media` | `-w` | Write output to a single MP3 file instead of playing. |
| `--save-dir` | `-s` | Directory to save chunked output files. |
| `--name` | `-n` | Sub-folder name for sequential saving (requires `-s`). |

## Rate Limiting

The tool automatically handles rate limiting to avoid any unfair use:
*   **Streaming:** ~8 chunks/min
*   **Saving:** ~20 chunks/min

These are predefined for now. Streaming (default mode) is lower because a natural voice at +80% speed is still slower than than the defined limit. The next chuck will be downloaded before you finish the previous chunks. 