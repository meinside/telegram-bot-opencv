# telegram-bot-opencv

A telegram bot for testing OpenCV scripts **remotely** and **headlessly**.

## you need:

- An OpenCV-ready machine (with camera)
- Golang installed
- A (python) script
- And a token of telegram bot.

## how to get & build:

```bash
$ git clone https://github.com/meinside/telegram-bot-opencv.git
$ cd telegram-bot-opencv/
$ go build
```

## how to configure:

Generate(or copy) a config file,

```bash
$ cp config.json.sample config.json
```

and replace values with yours:

```json
{
	"api_token": "0123456789:abcdefghijklmnopqrstuvwyz-x-0a1b2c3d4e",
	"allowed_ids": [
		"telegram_id_1",
		"telegram_id_2"
	],
	"monitor_interval": 5,
	"script_path": "/home/pi/python/opencv/detect_face.py",
	"is_verbose": false
}
```

## create a script:

Create a script in any programming language you like.

The script should print the result to STDOUT as one of the following formats:

- image
- video (.mp4)
- others

If image or video is given, bot will respond with it.

Otherwise, you'll get just a text message converted from the result.

### sample:

This is a python script which was tested on my Raspberry Pi with camera module:

```python
#!/usr/bin/env python
#
# Capture an image through rpi-camera, and detect faces in it.
# Print the result image to STDOUT as bytes.
#
# $ pip install "picamera[array]"

from picamera.array import PiRGBArray
from picamera import PiCamera
import time
import cv2

# https://raw.githubusercontent.com/shantnu/Webcam-Face-Detect/master/haarcascade_frontalface_default.xml
CASCADE_XML_FILEPATH = "/home/pi/python/opencv/haarcascade_frontalface_default.xml"

camera = PiCamera()
camera.resolution = (736, 480)

# warm-up
camera.start_preview()
time.sleep(0.1)

# capture in BGR format
stream = PiRGBArray(camera)
camera.capture(stream, format = "bgr")
image = stream.array

# convert BGR to grayscale
grayed = cv2.cvtColor(image, cv2.COLOR_BGR2GRAY)

# detect faces
face_cascade = cv2.CascadeClassifier(CASCADE_XML_FILEPATH)
faces = face_cascade.detectMultiScale(grayed, 1.1, 5)

# draw red rectangles over detected faces
for (x, y, w, h) in faces:
    cv2.rectangle(image, (x, y), (x + w, y + h), (0, 0, 255), 2)

# print bytes array of the image
print cv2.imencode('.jpg', image)[1].tostring()
```

It captures an image through Raspberry Pi camera module,

detect and mark faces in it, and print the final image to STDOUT.

The bot will execute this script, get bytes of the result, and respond back to the user.

![screen shot 2016-12-13 at 16 39 58](https://cloud.githubusercontent.com/assets/185988/21133426/162b97c6-c15c-11e6-8b4b-7ec9f1805829.png)

## run:

Execute the binary file:

```bash
$ ./telegram-bot-opencv
```

### run as service:

```bash
$ sudo cp systemd/telegram-bot-opencv.service /lib/systemd/system/
$ sudo vi /lib/systemd/system/telegram-bot-opencv.service
```

and edit **User**, **Group**, **WorkingDirectory** and **ExecStart** values.

It will launch automatically on boot with:

```bash
$ sudo systemctl enable telegram-bot-opencv.service
```

and will start/stop manually with:

```bash
$ sudo systemctl start telegram-bot-opencv.service
$ sudo systemctl stop telegram-bot-opencv.service
$ sudo systemctl restart telegram-bot-opencv.service
```

## license

MIT

