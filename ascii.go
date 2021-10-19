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
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"io/ioutil"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
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
	asciiWidth    = flag.Int("width", 250, "width in characters")
	asciiHeight   = flag.Int("height", 80, "height in characters")
	fontfile = flag.String("fontfile", "", "filename of a ttf font, preferably a monospaced one such as Courier")
	//file     = flag.String("file", "", "filename of the image file (supported are png and jpeg), if omitted current directory will be scanned for files, skip from image to image with enter")
	exact    = flag.Bool("exact", false, "require exact match for shade")
	mode     = flag.String("mode", "mono", "mode can be mono, gray or color, default is mono")
	alphabet = flag.String("alphabet", defaultAlphabet, "alphabet to use for art, if not set all printable ascii characters will be used")
	debug    = flag.Bool("debug", false, "if set to true some performance data will be printed")
	html     = flag.Bool("html", false, "output html instead of ascii")
	negative = flag.Bool("negative", true, "set to true if white text on black background, otherwise false")
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

func getHtmlColor(r, g, b int) string {
	r = 255 * r / maxColor
	g = 255 * g / maxColor
	b = 255 * b / maxColor
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "<font color='#%2x%2x%2x'>", r, g, b)
	return buf.String()
}

func getHtmlGray(r, g, b int) string {
	code := 255 * (r + g + b) / (3 * maxColor)
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "<font color='#%2x%2x%2x'>", code, code, code)
	return buf.String()
}

func getColor(r, g, b int, doHtml bool) string {
	if doHtml {
		return getHtmlColor(r, g, b)
	} else {
		return getXtermColor(r, g, b)
	}
}

func getGray(r, g, b int, doHtml bool) string {
	if doHtml {
		return getHtmlGray(r, g, b)
	} else {
		return getXtermGray(r, g, b)
	}
}

func getMono(r, g, b int, doHtml bool) string {
	if doHtml {
		if *negative {
			return getHtmlColor(maxColor, maxColor, maxColor)
		} else {
			return getHtmlColor(0, 0, 0)
		}
	} else {
		return ""
	}
}

func getResetColor(doHtml bool) string {
	if doHtml {
		return "</font>"
	}
	return ""
}

func analyzeImage(img *image.RGBA, doHtml bool) []string {
	defer trackTime(time.Now(), "analyze_image")
	numRows := *asciiWidth
	numLines := *asciiHeight
	boxWidth := (*img).Bounds().Dx() / numRows
	boxHeight := (*img).Bounds().Dy() / numLines
	min := 0
	max := maxColor * boxHeight * boxWidth
	var wait sync.WaitGroup
	ascii := make([][]ColorPoint, numLines)
	for l := 0; l < numLines; l++ {
		wait.Add(1)
		go func(l int) {
			ascii[l] = make([]ColorPoint, numRows)
			for o := 0; o < numRows; o++ {
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
	lines := make([]string, numLines)
	for l := 0; l < numLines; l++ {
		wait.Add(1)
		go func(l int) {
			var buffer bytes.Buffer
			for o := 0; o < numRows; o++ {
				normGS := int(256 * (ascii[l][o].sum - min) / (max - min))
				normR := ascii[l][o].r / (boxWidth * boxHeight)
				normG := ascii[l][o].g / (boxWidth * boxHeight)
				normB := ascii[l][o].b / (boxWidth * boxHeight)
				switch *mode {
				case "color":
					buffer.WriteString(getColor(normR, normG, normB, doHtml))
					break
				case "gray":
					buffer.WriteString(getGray(normR, normG, normB, doHtml))
					break
				case "mono":
					buffer.WriteString(getMono(normR, normG, normB, doHtml))
				}
				a := artifacts.FindClosest(normGS)
				if *exact && a.NormGS != normGS {
					buffer.WriteString(" ")
				} else {
					buffer.WriteString(a.Text)
				}
				switch *mode {
				case "color":
					buffer.WriteString(getResetColor(doHtml))
					break
				case "gray":
					buffer.WriteString(getResetColor(doHtml))
					break
				case "mono":
					buffer.WriteString(getResetColor(doHtml))
				}
			}
			lines[l] = buffer.String()
			wait.Done()
		}(l)
	}
	wait.Wait()
	return lines
}

func printASCII(w io.Writer, lines []string) {
	defer trackTime(time.Now(), "print_ascii")
	for _, line := range lines {
		fmt.Fprintf(w, "%s\n", line)
	}
	fmt.Fprint(w, resetTermColor)
}

func printHTML(w io.Writer, lines []string) {
	defer trackTime(time.Now(), "print_html")
	if *negative {
		fmt.Fprintf(w, "<html><body bgcolor='#000000'><p><font face='Courier' size='4'>\n")
	} else {
		fmt.Fprintf(w, "<html><body bgcolor='#ffffff'><p><font face='Courier' size='4'>\n")
	}
	for _, line := range lines {
		fmt.Fprintf(w, "%s<br/>\n", line)
	}
	fmt.Fprintf(w, "</font></p></body></html>\n")
}

func print(w io.Writer, lines []string, doHtml bool) {
	if doHtml {
		printHTML(w, lines)
	} else {
		printASCII(w, lines)
	}
}

func getImage(name string) *image.Image {
	defer trackTime(time.Now(), "get_image")
	var img image.Image
	f, err := os.Open(name)
	if err != nil {
		log.Fatal(err)
	}
	b := bufio.NewReader(f)
	if strings.HasSuffix(strings.ToLower(name), ".png") {
		img, err = png.Decode(b)
	} else if strings.HasSuffix(strings.ToLower(name), ".jpg") || strings.HasSuffix(strings.ToLower(name), ".jpeg") {
		img, err = jpeg.Decode(b)
	}
	if err != nil {
		log.Fatal(err)
	}
	return &img
}

func trackTime(start time.Time, name string) {
	elapsed := time.Since(start)
	if *debug {
		log.Printf("event=%s duration=%s", name, elapsed)
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
	defer trackTime(time.Now(), "analyze_font")
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
	artifacts = loadCharacterMap()
}

/*func main() {
	flag.Parse()
	if *fontfile == "" {
		if *alphabet != defaultAlphabet || !(*negative) {
			*fontfile = defaultTtfFile
		}
	}
	if *fontfile != "" {
		artifacts = analyzeFont(*fontfile)
	} else {
		artifacts = loadCharacterMap()
	}
	if *file != "" {
		print(os.Stdout, analyzeImage(getImage(*file), *html), *html)
	} else {
		fileInfos, err := ioutil.ReadDir("./")
		if err != nil {
			log.Fatal(err)
		}
		imageFiles := make([]string, 0)
		for _, info := range fileInfos {
			if strings.HasSuffix(strings.ToLower(info.Name()), ".png") || strings.HasSuffix(strings.ToLower(info.Name()), ".jpg") || strings.HasSuffix(strings.ToLower(info.Name()), ".jpeg") {
				imageFiles = append(imageFiles, info.Name())
			}
		}
		if len(imageFiles) == 0 {
			return
		}
		img := getImage(imageFiles[0])
		for i := 1; i < len(imageFiles); i++ {
			fmt.Printf("file=%s\n", imageFiles[i-1])
			print(os.Stdout, analyzeImage(img, *html), *html)
			img = getImage(imageFiles[i])
			s := ""
			fmt.Scanf("%s", &s)
		}
		fmt.Printf("file=%s\n", imageFiles[len(imageFiles)-1])
		print(os.Stdout, analyzeImage(img, *html), *html)
	}
}*/
