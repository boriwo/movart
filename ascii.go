/**
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"io"
	"io/ioutil"
	"log"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/golang/freetype"
	"github.com/golang/freetype/truetype"
)

const (
	defaultAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcedefghijklmnopqrstuvwxyz!@#$%^&*()-_=+1234567890[]{};':\",.<>/?\\|~` "
	resetTermColor  = "\x1B[0m"
	maxColor        = 65536
	mappingFile     = "artifacts.json"
	defaultTtfFile  = "courier_prime.ttf"
)

var (
)

var (
	artifacts SortedGS
)

type (
	Artifact struct {
		Text          string
		AbsGS, NormGS int
	}
	SortedGS   []*Artifact
	ColorPoint struct {
		r, g, b, sum int
	}
)

func (a Artifact) String() string {
	return fmt.Sprintf("%s\t%d\t%d", a.Text, a.AbsGS, a.NormGS)
}

func (artifacts SortedGS) String() string {
	s := ""
	for _, a := range artifacts {
		s = s + fmt.Sprintf("%s\t%d\t%d\n", a.Text, a.AbsGS, a.NormGS)
	}
	return s
}

func (artifacts SortedGS) Normalize() {
	max := -1
	min := -1
	for _, a := range artifacts {
		gs := a.AbsGS
		if gs > max {
			max = gs
		}
		if min == -1 || gs < min {
			min = gs
		}
	}
	for _, a := range artifacts {
		a.NormGS = 256 * (a.AbsGS - min) / (max - min)
	}
}

func (artifacts SortedGS) FindClosest(gs int) *Artifact {
	l := 0
	r := len(artifacts) - 1
	for {
		m := (l + r) / 2
		if m == l || m == r {
			return artifacts[m]
		}
		if artifacts[m].NormGS > gs {
			r = m
		} else {
			l = m
		}
	}
}

func (a SortedGS) Len() int           { return len(a) }
func (a SortedGS) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a SortedGS) Less(i, j int) bool { return a[i].AbsGS < a[j].AbsGS }

func removeDuplicates(aList SortedGS) SortedGS {
	result := SortedGS{}
	for i, a := range aList {
		if i > 0 && a.NormGS == aList[i-1].NormGS {
			continue
		}
		result = append(result, a)
	}
	return result
}

func mono(v uint32) int {
	if v > maxColor/2 {
		return 1
	} else {
		return 0
	}
}

func getXtermColor(r, g, b int) string {
	code := 36*int(5*float64(r)/maxColor) + 6*int(5*float64(g)/maxColor) + int(5*float64(b)/maxColor) + 16
	return "\x1B[38;5;" + strconv.Itoa(code) + "m"
}

func getXtermGray(r, g, b int) string {
	code := 232 + (255-232)*(r+g+b)/(3*maxColor)
	return "\x1B[38;5;" + strconv.Itoa(code) + "m"
}

func getColor(r, g, b int) string {
		return getXtermColor(r, g, b)
}

func getGray(r, g, b int) string {
		return getXtermGray(r, g, b)
}

func allocateAsciiArray() [][]ColorPoint{
	numRows := *asciiWidth
	numLines := *asciiHeight
	ascii := make([][]ColorPoint, numLines)
	for l := 0; l < numLines; l++ {
		ascii[l] = make([]ColorPoint, numRows)
	}
	return ascii
}

func analyzeImage(img *image.RGBA, ascii [][]ColorPoint, lines []string) []string {
	lock.Lock()
	defer lock.Unlock()
	defer trackTime(time.Now(), "analyze_image", 5, *asciiHeight-4, lines)
	numRows := *asciiWidth
	numLines := *asciiHeight
	boxWidth := (*img).Bounds().Dx() / numRows
	boxHeight := (*img).Bounds().Dy() / numLines
	min := 0
	max := maxColor * boxHeight * boxWidth
	var wait sync.WaitGroup
	for l := 0; l < numLines; l++ {
		wait.Add(1)
		go func(l int) {
			for o := 0; o < numRows; o++ {
				ascii[l][o].r = 0
				ascii[l][o].g = 0
				ascii[l][o].b = 0
				ascii[l][o].sum = 0
				for y := 0; y < boxHeight; y++ {
					for x := 0; x < boxWidth; x++ {
						col := (*img).At(o*boxWidth+x, l*boxHeight+y)
						r, g, b, _ := col.RGBA()
						ascii[l][o].r += int(r)
						ascii[l][o].g += int(g)
						ascii[l][o].b += int(b)
						ascii[l][o].sum += (ascii[l][o].r + ascii[l][o].g + ascii[l][o].b)
					}
				}
				if ascii[l][o].sum < min {
					min = ascii[l][o].sum
				} else if ascii[l][o].sum > max {
					max = ascii[l][o].sum
				}
			}
			wait.Done()
		}(l)
	}
	wait.Wait()
	lastNormRGB := 0
	for l := 0; l < numLines; l++ {
		wait.Add(1)
		go func(l int) {
			var buffer bytes.Buffer
			for o := 0; o < numRows; o++ {
				normGS := int(256 * (ascii[l][o].sum - min) / (max - min))
				normR := ascii[l][o].r / (boxWidth * boxHeight)
				normG := ascii[l][o].g / (boxWidth * boxHeight)
				normB := ascii[l][o].b / (boxWidth * boxHeight)
				a := artifacts.FindClosest(normGS)
				if a.Text != " " {
						switch *mode {
						case "color":
							if lastNormRGB != normR+normG+normG {
								buffer.WriteString(getColor(normR, normG, normB))
							}
							break
						case "gray":
							if lastNormRGB != normR+normG+normG {
								buffer.WriteString(getGray(normR, normG, normB))
							}
							break
						}
				}
				if *exact && a.NormGS != normGS {
					buffer.WriteString(" ")
				} else {
					buffer.WriteString(a.Text)
				}
				lastNormRGB = normR + normG + normB
			}
			lines[l] = buffer.String()
			wait.Done()
		}(l)
	}
	wait.Wait()
	return lines
}

func printASCII(w io.Writer, lines []string) {
	now := time.Now()
	// ansi escape codes
	//fmt.Print("\033[2J") // clear screen
	fmt.Printf("\033[%d;%dH", 0, 0) // set cursor position
	fmt.Print("\033[2~")            // insert mode
	for idx, _ := range lines {
		if idx == *asciiHeight-3 {
			trackTime(now, "print_ascii", 5, *asciiHeight-3, lines)
		}
		fmt.Fprintf(w, "%s\n", lines[idx])
	}
	fmt.Fprint(w, resetTermColor)
}

func print(w io.Writer, lines []string) {
	if player.GetFrameIdx() % *showNthFrame == 0 {
		printASCII(w, lines)
	}
}

func trackTime(start time.Time, name string, x, y int, lines []string) {
	if *debug {
		elapsed := time.Since(start)
		str := fmt.Sprintf("event=%s duration=%s frame=%d frameBufferDepth=%d sampleBufferDepth=%d                                                        ",
			name, elapsed, player.GetFrameIdx(), player.GetFrameBufferDepth(), player.GetSampleBufferDepth())
		lines[y] = lines[y][0:x-1] + str + lines[y][x+len(str):]
	}
}

func getNumBlackPixels(rgba *image.RGBA) int {
	s := 0
	for y := 0; y < rgba.Bounds().Dy(); y++ {
		for x := 0; x < rgba.Bounds().Dx(); x++ {
			col := rgba.At(x, y)
			r, g, b, _ := col.RGBA()
			s += (mono(r) + mono(g) + mono(b))
		}
	}
	if *negative {
		return 3*(rgba.Bounds().Dy()*rgba.Bounds().Dx()) - s
	} else {
		return s
	}
}

func getRGBA(str string, font *truetype.Font) *image.RGBA {
	rgba := image.NewRGBA(image.Rect(0, 0, 18, 18))
	draw.Draw(rgba, rgba.Bounds(), image.White, image.ZP, draw.Src)
	c := freetype.NewContext()
	c.SetDPI(150)
	c.SetFont(font)
	c.SetFontSize(14)
	c.SetClip(rgba.Bounds())
	c.SetDst(rgba)
	c.SetSrc(image.Black)
	//c.SetHinting(freetype.NoHinting)
	pt := freetype.Pt(0, 18)
	_, err := c.DrawString(str, pt)
	if err != nil {
		log.Fatal(err)
	}
	return rgba
}

func analyzeFont(ttfFile string) SortedGS {
	f, err := ioutil.ReadFile(ttfFile)
	if err != nil {
		log.Fatal(err)
	}
	font, err := truetype.Parse(f)
	if err != nil {
		log.Fatal(err)
	}
	a := make(SortedGS, len(*alphabet))
	for i, char := range *alphabet {
		rgba := getRGBA(string(char), font)
		nbp := getNumBlackPixels(rgba)
		a[i] = &Artifact{string(char), nbp, nbp}
	}
	a.Normalize()
	sort.Sort(a)
	a = removeDuplicates(a)
	//saveCharacterMap(a)
	return a
}

func saveCharacterMap(a SortedGS) {
	buf, err := json.Marshal(a)
	if err != nil {
		log.Fatal(err)
	}
	err = ioutil.WriteFile(mappingFile, buf, 0644)
	if err != nil {
		log.Fatal(err)
	}
}

func loadCharacterMap() SortedGS {
	buf, err := ioutil.ReadFile(mappingFile)
	if err != nil {
		log.Fatal(err)
	}
	var a SortedGS
	if err := json.Unmarshal(buf, &a); err != nil {
		log.Fatal(err)
	}
	return a
}

func initAscii() {
	ascii = allocateAsciiArray()
	lines = make([]string, *asciiHeight)
	if *fontfile != "" {
		artifacts = analyzeFont(*fontfile)
	} else {
		artifacts = loadCharacterMap()
	}
}
