# movart

This work is based on previous work [here](https://github.com/boriwo/art). While art does only ASCII conversion
for static images, movart will do the same for mp4 encoded videos. Just like before you can choose between monochrome,
gray scale and color characters. Movart also supports playing the audio stream if desired using the -audio option.

![madness.png](madness.png)

Example Usage:

```
./movart -file video.mp4

./movart -file video.mp4 -audio

./movart -file video.mp4 -audio

./movart -file video.mp4 -mode gray

./movart -file video.mp4 -mode color

./movart -file video.mp4 -alphabet "*\. " -fontfile courier_prime.ttf
```

###Libraries used for this project:

_Reisen_

A simple library to extract video and audio frames from media containers.

https://golangrepo.com/repo/zergon321-reisen-go-video

https://github.com/zergon321/reisen

_Ebiten_

2D Game Engine

https://github.com/hajimehoshi/ebiten

_Disintegration Imaging Library_

https://github.com/disintegration/imaging

### Other useful reading:

_OpenCV 4 Computer Vision Library_

https://github.com/hybridgroup/gocv









