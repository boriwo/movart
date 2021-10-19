package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"github.com/faiface/beep"
	"github.com/hajimehoshi/ebiten"
	"github.com/zergon321/reisen"
	"image"
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
	//width                             = 1280
	//height                            = 546
	filename = flag.String("file", "demo.mp4", "media file name")
	asciiWidth    = flag.Int("width", 250, "width in characters")
	asciiHeight   = flag.Int("height", 80, "height in characters")
	fontfile = flag.String("fontfile", "", "filename of a ttf font, preferably a monospaced one such as Courier")
	exact    = flag.Bool("exact", false, "require exact match for shade")
	mode     = flag.String("mode", "mono", "mode can be mono, gray or color, default is mono")
	alphabet = flag.String("alphabet", defaultAlphabet, "alphabet to use for art, if not set all printable ascii characters will be used")
	debug    = flag.Bool("debug", false, "if set to true some performance data will be printed")
	negative = flag.Bool("negative", true, "set to true if white text on black background, otherwise false")
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

func main() {
	flag.Parse()
	initAscii()
	player := &Player{}
	err := player.Start(*filename)
	handleError(err)
	ebiten.SetWindowSize(width, height)
	ebiten.SetWindowTitle("Video")
	err = ebiten.RunGame(player)
	handleError(err)
}

func handleError(err error) {
	if err != nil {
		panic(err)
	}
}
