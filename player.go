package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/faiface/beep"
	"github.com/faiface/beep/speaker"
	"github.com/zergon321/reisen"
	"image"
	"os"
	"time"
)

// Player holds all the data
// necessary for playing video.
type Player struct {
	ticker                 <-chan time.Time
	errs                   <-chan error
	frameBuffer            <-chan *image.RGBA
	fps                    int
	videoTotalFramesPlayed int
	videoPlaybackFPS       int
	perSecond              <-chan time.Time
	last                   time.Time
	deltaTime              float64
	sampleBuffer <-chan [2]float64
}

func (player *Player) GetFrameBufferDepth() int {
	return len(player.frameBuffer)
}

func (player *Player) GetSampleBufferDepth() int {
	return len(player.sampleBuffer)
}

func (player *Player) GetFrameIdx() int {
	return player.videoTotalFramesPlayed
}

func (player *Player) Render() error {
	player.deltaTime = time.Since(player.last).Seconds()
	player.last = time.Now()
	select {
	case err, ok := <-player.errs:
		if ok {
			return err
		}
	default:
	}
	select {
	case <-player.ticker:
		frame, ok := <-player.frameBuffer
		if ok {
			// asciify image
			asciiLines := analyzeImage(frame, ascii, lines)
			print(os.Stdout, asciiLines)
			player.videoTotalFramesPlayed++
			player.videoPlaybackFPS++
		}
	default:
	}
	// draw the video sprite
	player.fps++
	// Update metrics in the window title.
	select {
	case <-player.perSecond:
		// set title, print stats
		player.fps = 0
		player.videoPlaybackFPS = 0
	default:
	}
	return nil
}

func (player *Player) Layout(a, b int) (int, int) {
	return width, height
}

func (player *Player) Start(fname string) error {
	// find frame rate
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
	frameDuration, err := time.ParseDuration(fmt.Sprintf("%fs", spf))
	if err != nil {
		return err
	}
	// open streams
	err = media.OpenDecode()
	if err != nil {
		return err
	}
	videoStreams :=  media.VideoStreams()
	videoStream := videoStreams[0]
	err = videoStream.Open()
	if err != nil {
		return err
	}
	width = videoStream.Width()
	height = videoStream.Height()
	audioStream := media.AudioStreams()[0]
	// init speaker
	err = audioStream.Open()
	if err != nil {
		return err
	}
	sampleRate = audioStream.SampleRate()
	if *playAudio {
		err := speaker.Init(beep.SampleRate(sampleRate), SpeakerSampleRate.N(time.Second/10))
		if err != nil {
			return err
		}
	}
	// start decoding streams
	player.frameBuffer, player.sampleBuffer, player.errs, err = readVideoAndAudio(media, videoStream, audioStream)
	if err != nil {
		return err
	}
	// start playing audio samples
	if *playAudio {
		speaker.Play(streamSamples(player.sampleBuffer))
	}
	// setup metrics
	player.last = time.Now()
	player.fps = 0
	player.videoTotalFramesPlayed = 0
	player.videoPlaybackFPS = 0
	player.ticker = time.Tick(frameDuration)
	player.perSecond = time.Tick(time.Second)
	return nil
}

// readVideoAndAudio reads video and audio frames
// from the opened media and sends the decoded
// data to che channels to be played.
func readVideoAndAudio(media *reisen.Media, videoStream *reisen.VideoStream, audioStream *reisen.AudioStream) (<-chan *image.RGBA, <-chan [2]float64, chan error, error) {
	frameBuffer := make(chan *image.RGBA, frameBufferSize)
	sampleBuffer := make(chan [2]float64, sampleBufferSize)
	errs := make(chan error)
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
				// flip image if needed
				// flip image if needed
				//flippedImage := imaging.FlipV(videoFrame.Image())
				//bounds := flippedImage.Bounds()
				//flippedImageRGBA := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
				//draw.Draw(flippedImageRGBA, flippedImageRGBA.Bounds(), flippedImage, bounds.Min, draw.Src)
				//frameBuffer <- flippedImageRGBA
				frameBuffer <- videoFrame.Image()
			case reisen.StreamAudio:
				if !*playAudio {
					continue
				}
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
				// turn the raw byte data into audio samples of type [2]float64.
				reader := bytes.NewReader(audioFrame.Data())
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


