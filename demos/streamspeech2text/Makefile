all:
	ls *.mp3 | entr play output.mp3 &
	go build && gst-launch-1.0 -v pulsesrc ! audioconvert ! audioresample ! audio/x-raw,channels=1,rate=16000 ! filesink location=/dev/stdout | ./streamspeech2text
