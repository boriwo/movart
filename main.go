package main

import (
	"flag"
	"github.com/faiface/beep"
	"sync"
	"time"
)

const (
	frameBufferSize                   = 1024
	channelCount                      = 2
	bitDepth                          = 8
	sampleBufferSize                  = 32 * channelCount * bitDepth * 1024
	SpeakerSampleRate beep.SampleRate = 44100
)

var (
	sampleRate                        = 44100
	width                             = 1280
	height                            = 720
	playAudio = flag.Bool("audio", false, "play audio stream")
	filename = flag.String("file", "demo.mp4", "media file name")
	asciiWidth    = flag.Int("width", 250, "width in characters")
	asciiHeight   = flag.Int("height", 80, "height in characters")
	fontfile = flag.String("fontfile", "", "filename of a ttf font, preferably a monospaced one such as Courier")
	exact    = flag.Bool("exact", false, "require exact match for shade")
	mode     = flag.String("mode", "mono", "mode can be mono, gray or color, default is mono")
	alphabet = flag.String("alphabet", defaultAlphabet, "alphabet to use for art, if not set all printable ascii characters will be used")
	debug    = flag.Bool("debug", false, "if set to true some performance data will be printed")
	negative = flag.Bool("negative", true, "set to true if white text on black background, otherwise false")
	showNthFrame = flag.Int("snf", 2, "only show every nth frame, default is 2, meaning only show every second frame to ensure frame buffer doesn't back up")
	ascii [][]ColorPoint
	lines []string
	lock sync.RWMutex
	player *Player
)

func main() {
	flag.Parse()
	initAscii()
	player = &Player{}
	err := player.Start(*filename)
	handleError(err)
	for {
		player.Render()
		time.Sleep(2*time.Millisecond)
	}
}

func handleError(err error) {
	if err != nil {
		panic(err)
	}
}
