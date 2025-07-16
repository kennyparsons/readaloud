# readaloud

`readaloud` is a command-line tool for text-to-speech synthesis using Microsoft Edge's online service.

## Installation

```bash
go install github.com/kennyparsons/readaloud
```

## Usage

To play the synthesized speech directly:

```bash
./readaloud "Hello, world!"
```

To write the output to a media file:

```bash
./readaloud "Hello, world!" --write-media hello.mp3
```

To pass clipboard content to the tool:

```bash
pbpaste | ./readaloud
```