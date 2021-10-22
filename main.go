package main

import (
	"flag"
	"github.com/faiface/beep"
)

const (
	frameBufferSize                   = 1024
	channelCount                      = 2
	bitDepth                          = 8
	sampleBufferSize                  = 32 * channelCount * bitDepth * 1024
	SpeakerSampleRate beep.SampleRate = 44100
	defaultSampleRate              	  = 44100
	defaultWidth                      = 1280
	defaultHeight                     = 720

)

var (
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
	fps = flag.Int("fps", 0, "optionally overwrite frame rate information in video stream")
	player *Player
	ascii *Ascii
)

func main() {
	flag.Parse()
	ascii = NewAscii(*alphabet, *mode, *asciiHeight, *asciiWidth, *exact, *negative, *debug)
	player = NewPlayer(defaultWidth, defaultHeight, defaultSampleRate, *showNthFrame, *fps)
	err := player.Start(*filename)
	handleError(err)
	for {
		player.Render()
		//time.Sleep(2*time.Millisecond)
	}
}

func handleError(err error) {
	if err != nil {
		panic(err)
	}
}


