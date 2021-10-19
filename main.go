package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"os"
	"time"

	"github.com/faiface/beep"
	"github.com/faiface/beep/speaker"
	"github.com/hajimehoshi/ebiten"
	"github.com/zergon321/reisen"
)

const (
	frameBufferSize                   = 1024
	sampleRate                        = 44100
	channelCount                      = 2
	bitDepth                          = 8
	sampleBufferSize                  = 32 * channelCount * bitDepth * 1024
	SpeakerSampleRate beep.SampleRate = 44100
)

var (
	//width                             = 1920
	//height                            = 1080
	width                             = 1280
	height                            = 720
)

// readVideoAndAudio reads video and audio frames
// from the opened media and sends the decoded
// data to che channels to be played.
func readVideoAndAudio(media *reisen.Media) (<-chan *image.RGBA, <-chan [2]float64, chan error, error) {
	frameBuffer := make(chan *image.RGBA, frameBufferSize)
	sampleBuffer := make(chan [2]float64, sampleBufferSize)
	errs := make(chan error)
	err := media.OpenDecode()
	if err != nil {
		return nil, nil, nil, err
	}
	videoStreams :=  media.VideoStreams()
	videoStream := videoStreams[0]
	err = videoStream.Open()
	if err != nil {
		return nil, nil, nil, err
	}
	width = videoStream.Width()
	height = videoStream.Height()
	audioStream := media.AudioStreams()[0]
	err = audioStream.Open()
	if err != nil {
		return nil, nil, nil, err
	}
	go func() {
		for {
			packet, gotPacket, err := media.ReadPacket()
			if err != nil {
				go func(err error) {
					errs <- err
				}(err)
			}
			if !gotPacket {
				break
			}
			//TODO: make sure audio and video stays in sync
			switch packet.Type() {
			case reisen.StreamVideo:
				s := media.Streams()[packet.StreamIndex()].(*reisen.VideoStream)
				videoFrame, gotFrame, err := s.ReadVideoFrame()
				if err != nil {
					continue
					/*go func(err error) {
						errs <- err
					}(err)*/
				}
				if !gotFrame {
					continue
					//break
				}
				if videoFrame == nil {
					continue
				}
				// flip image
				//flippedImage := imaging.FlipV(videoFrame.Image())
				//bounds := flippedImage.Bounds()
				//flippedImageRGBA := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
				//draw.Draw(flippedImageRGBA, flippedImageRGBA.Bounds(), flippedImage, bounds.Min, draw.Src)
				//frameBuffer <- flippedImageRGBA
				frameBuffer <- videoFrame.Image()
			case reisen.StreamAudio:
				s := media.Streams()[packet.StreamIndex()].(*reisen.AudioStream)
				audioFrame, gotFrame, err := s.ReadAudioFrame()
				if err != nil {
					continue
					/*go func(err error) {
						errs <- err
					}(err)*/
				}
				if !gotFrame {
					continue
					//break
				}
				if audioFrame == nil {
					continue
				}
				// Turn the raw byte data into
				// audio samples of type [2]float64.
				reader := bytes.NewReader(audioFrame.Data())
				// See the README.md file for
				// detailed scheme of the sample structure.
				for reader.Len() > 0 {
					sample := [2]float64{0, 0}
					var result float64
					err = binary.Read(reader, binary.LittleEndian, &result)
					if err != nil {
						go func(err error) {
							errs <- err
						}(err)
					}
					sample[0] = result
					err = binary.Read(reader, binary.LittleEndian, &result)
					if err != nil {
						go func(err error) {
							errs <- err
						}(err)
					}
					sample[1] = result
					sampleBuffer <- sample
				}
			}
		}
		videoStream.Close()
		audioStream.Close()
		media.CloseDecode()
		close(frameBuffer)
		close(sampleBuffer)
		close(errs)
	}()
	return frameBuffer, sampleBuffer, errs, nil
}

// streamSamples creates a new custom streamer for
// playing audio samples provided by the source channel.
//
// See https://github.com/faiface/beep/wiki/Making-own-streamers
// for reference.
func streamSamples(sampleSource <-chan [2]float64) beep.Streamer {
	return beep.StreamerFunc(func(samples [][2]float64) (n int, ok bool) {
		numRead := 0
		for i := 0; i < len(samples); i++ {
			sample, ok := <-sampleSource
			if !ok {
				numRead = i + 1
				break
			}
			samples[i] = sample
			numRead++
		}
		if numRead < len(samples) {
			return numRead, false
		}
		return numRead, true
	})
}

// Game holds all the data
// necessary for playing video.
type Game struct {
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

// Strarts reading samples and frames
// of the media file.
func (game *Game) Start(fname string) error {
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
	game.videoSprite, err = ebiten.NewImage(width, height, ebiten.FilterDefault)
	game.frameBuffer, sampleSource, game.errs, err = readVideoAndAudio(media)
	if err != nil {
		return err
	}
	// Start playing audio samples.
	speaker.Play(streamSamples(sampleSource))
	game.ticker = time.Tick(frameDuration)
	// Setup metrics.
	game.last = time.Now()
	game.fps = 0
	game.perSecond = time.Tick(time.Second)
	game.videoTotalFramesPlayed = 0
	game.videoPlaybackFPS = 0
	return nil
}

func (game *Game) Update(screen *ebiten.Image) error {
	// Compute dt.
	game.deltaTime = time.Since(game.last).Seconds()
	game.last = time.Now()
	// Check for incoming errors.
	select {
	case err, ok := <-game.errs:
		if ok {
			return err
		}
	default:
	}
	// Read video frames and draw them.
	select {
	case <-game.ticker:
		frame, ok := <-game.frameBuffer
		if ok {
			// asciify image
			// ansi escape codes
			//fmt.Print("\033[2J") // clear screen
			fmt.Printf("\033[%d;%dH", 0, 0) // set cursor position
			fmt.Print("\033[2~")            // insert mode
			asciiLines := analyzeImage(frame, false)
			print(os.Stdout, asciiLines, false)
			game.videoSprite.ReplacePixels(frame.Pix)
			game.videoTotalFramesPlayed++
			game.videoPlaybackFPS++
		}
	default:
	}
	// Draw the video sprite.
	op := &ebiten.DrawImageOptions{}
	err := screen.DrawImage(game.videoSprite, op)
	if err != nil {
		return err
	}
	game.fps++
	// Update metrics in the window title.
	select {
	case <-game.perSecond:
		ebiten.SetWindowTitle(fmt.Sprintf("%s | FPS: %d | dt: %f | Frames: %d | Video FPS: %d",
			"Video", game.fps, game.deltaTime, game.videoTotalFramesPlayed, game.videoPlaybackFPS))
		game.fps = 0
		game.videoPlaybackFPS = 0
	default:
	}
	return nil
}

func (game *Game) Layout(a, b int) (int, int) {
	return width, height
}

func main() {
	filename := flag.String("file", "demo.mp4", "media file name")
	flag.Parse()
	initAscii()
	game := &Game{}
	err := game.Start(*filename)
	handleError(err)
	ebiten.SetWindowSize(width, height)
	ebiten.SetWindowTitle("Video")
	err = ebiten.RunGame(game)
	handleError(err)
}

func handleError(err error) {
	if err != nil {
		panic(err)
	}
}
