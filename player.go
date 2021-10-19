package main

import (
	"fmt"
	"github.com/faiface/beep/speaker"
	"github.com/hajimehoshi/ebiten"
	"github.com/zergon321/reisen"
	"image"
	"os"
	"time"
)

// Player holds all the data
// necessary for playing video.
type Player struct {
	videoSprite            *ebiten.Image
	ticker                 <-chan time.Time
	errs                   <-chan error
	frameBuffer            <-chan *image.RGBA
	fps                    int
	videoTotalFramesPlayed int
	videoPlaybackFPS       int
	perSecond              <-chan time.Time
	last                   time.Time
	deltaTime              float64
}

func (player *Player) Update(screen *ebiten.Image) error {
	// Compute dt.
	player.deltaTime = time.Since(player.last).Seconds()
	player.last = time.Now()
	// Check for incoming errors.
	select {
	case err, ok := <-player.errs:
		if ok {
			return err
		}
	default:
	}
	// Read video frames and draw them.
	select {
	case <-player.ticker:
		frame, ok := <-player.frameBuffer
		if ok {
			// asciify image
			// ansi escape codes
			//fmt.Print("\033[2J") // clear screen
			fmt.Printf("\033[%d;%dH", 0, 0) // set cursor position
			fmt.Print("\033[2~")            // insert mode
			asciiLines := analyzeImage(frame)
			print(os.Stdout, asciiLines)
			player.videoSprite.ReplacePixels(frame.Pix)
			player.videoTotalFramesPlayed++
			player.videoPlaybackFPS++
		}
	default:
	}
	// Draw the video sprite.
	op := &ebiten.DrawImageOptions{}
	err := screen.DrawImage(player.videoSprite, op)
	if err != nil {
		return err
	}
	player.fps++
	// Update metrics in the window title.
	select {
	case <-player.perSecond:
		ebiten.SetWindowTitle(fmt.Sprintf("%s | FPS: %d | dt: %f | Frames: %d | Video FPS: %d",
			"Video", player.fps, player.deltaTime, player.videoTotalFramesPlayed, player.videoPlaybackFPS))
		player.fps = 0
		player.videoPlaybackFPS = 0
	default:
	}
	return nil
}

func (player *Player) Layout(a, b int) (int, int) {
	return width, height
}

// Strarts reading samples and frames
// of the media file.
func (player *Player) Start(fname string) error {
	// Initialize the audio speaker.
	err := speaker.Init(sampleRate, SpeakerSampleRate.N(time.Second/10))
	if err != nil {
		return err
	}
	if err != nil {
		return err
	}
	// Open the media file.
	media, err := reisen.NewMedia(fname)
	if err != nil {
		return err
	}
	spf := float64(1.0)
	var frameRateNum, freameRateDen int
	for _, stream := range media.Streams() {
		if stream.Type() == reisen.StreamVideo {
			frameRateNum, freameRateDen = stream.FrameRate()
			spf = 1.0/float64(frameRateNum/freameRateDen) // "seconds per frame"
		}
	}
	fmt.Printf("video fpr %d %d\n", frameRateNum, freameRateDen)
	frameDuration, err := time.ParseDuration(fmt.Sprintf("%fs", spf))
	if err != nil {
		return err
	}
	// Start decoding streams.
	var sampleSource <-chan [2]float64
	// Sprite for drawing video frames.
	player.videoSprite, err = ebiten.NewImage(width, height, ebiten.FilterDefault)
	player.frameBuffer, sampleSource, player.errs, err = readVideoAndAudio(media)
	if err != nil {
		return err
	}
	// Start playing audio samples.
	speaker.Play(streamSamples(sampleSource))
	player.ticker = time.Tick(frameDuration)
	// Setup metrics.
	player.last = time.Now()
	player.fps = 0
	player.perSecond = time.Tick(time.Second)
	player.videoTotalFramesPlayed = 0
	player.videoPlaybackFPS = 0
	return nil
}

