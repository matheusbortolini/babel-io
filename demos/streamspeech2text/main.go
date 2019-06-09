// Copyright 2019 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Command livecaption pipes the stdin audio data to
// Google Speech API and outputs the transcript.
//
// As an example, gst-launch can be used to capture the mic input:
//
//    $ gst-launch-1.0 -v pulsesrc ! audioconvert ! audioresample ! audio/x-raw,channels=1,rate=16000 ! filesink location=/dev/stdout | livecaption
package main

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"

	speech "cloud.google.com/go/speech/apiv1"
	texttospeech "cloud.google.com/go/texttospeech/apiv1"
	"cloud.google.com/go/translate"
	"golang.org/x/text/language"
	speechpb "google.golang.org/genproto/googleapis/cloud/speech/v1"
	texttospeechpb "google.golang.org/genproto/googleapis/cloud/texttospeech/v1"
)

var (
	speechClient *speech.Client
)

func setupSpeechStream(ctx context.Context) (speechpb.Speech_StreamingRecognizeClient, error) {
	speechClient, err := speech.NewClient(ctx)
	if err != nil {
		return nil, err
	}
	stream, err := speechClient.StreamingRecognize(ctx)
	if err != nil {
		return nil, err
	}
	// Send the initial configuration message.
	err = stream.Send(&speechpb.StreamingRecognizeRequest{
		StreamingRequest: &speechpb.StreamingRecognizeRequest_StreamingConfig{
			StreamingConfig: &speechpb.StreamingRecognitionConfig{
				Config: &speechpb.RecognitionConfig{
					Encoding:                   speechpb.RecognitionConfig_LINEAR16,
					SampleRateHertz:            16000,
					LanguageCode:               "en-US",
					EnableAutomaticPunctuation: true,
				},
			},
		},
	})

	return stream, err
}

func startListeningStdin(stream speechpb.Speech_StreamingRecognizeClient) {
	// Pipe stdin to the API.
	buf := make([]byte, 1024)
	for {
		n, err := os.Stdin.Read(buf)
		if err := stream.Send(&speechpb.StreamingRecognizeRequest{
			StreamingRequest: &speechpb.StreamingRecognizeRequest_AudioContent{
				AudioContent: buf[:n],
			},
		}); err != nil {
			log.Printf("Could not send audio: %v", err)
		}
		if err == io.EOF {
			// Nothing else to pipe, close the stream.
			if err := stream.CloseSend(); err != nil {
				log.Fatalf("Could not close stream: %v", err)
			}
			return
		}
		if err != nil {
			log.Printf("Could not read from stdin: %v", err)
			continue
		}
	}
}

func startReceivingStream(stream speechpb.Speech_StreamingRecognizeClient,
	alts chan *speechpb.SpeechRecognitionAlternative) {
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatalf("Cannot stream results: %v", err)
		}
		if err := resp.Error; err != nil {
			// Workaround while the API doesn't give a more informative error.
			if err.Code == 3 || err.Code == 11 {
				log.Print("WARNING: Speech recognition request exceeded limit of 60 seconds.")
			}
			log.Fatalf("Could not recognize: %v", err)
		}
		for _, result := range resp.Results {
			alternatives := result.GetAlternatives()
			for _, alt := range alternatives {
				fmt.Println("Transcript alternatives: ", alt.Transcript)
				alts <- alt
			}
		}
	}
}

func main() {
	ctx := context.Background()

	stream, err := setupSpeechStream(ctx)
	if err != nil {
		panic(err)
	}
	alts := make(chan *speechpb.SpeechRecognitionAlternative)
	texts := make(chan string)

	go startListeningStdin(stream)
	go startReceivingStream(stream, alts)
	go startTranslating(alts, "pt-BR", texts)
	go startSpeaking(texts, "pt-BR")

	wait := make(chan interface{})
	<-wait
}

func startTranslating(alts chan *speechpb.SpeechRecognitionAlternative, code string, texts chan string) {
	ctx := context.Background()

	for {
		text := (<-alts).Transcript
		// Creates a client.
		client, err := translate.NewClient(ctx)
		if err != nil {
			log.Fatalf("Failed to create client: %v", err)
		}

		// Sets the text to translate.
		target, err := language.Parse(code)
		if err != nil {
			log.Fatalf("Failed to parse target language: %v", err)
		}

		// Translates the text into Russian.
		translations, err := client.Translate(ctx, []string{text}, target, nil)
		if err != nil {
			log.Fatalf("Failed to translate text: %v", err)
		}

		fmt.Printf("Translation: %v\n", translations[0].Text)
		texts <- translations[0].Text
	}
}

func startSpeaking(texts chan string, lang string) {
	// Instantiates a client.
	ctx := context.Background()

	client, err := texttospeech.NewClient(ctx)
	if err != nil {
		log.Fatal(err)
	}
	for {
		text := <-texts

		// Perform the text-to-speech request on the text input with the selected
		// voice parameters and audio file type.
		req := texttospeechpb.SynthesizeSpeechRequest{
			// Set the text input to be synthesized.
			Input: &texttospeechpb.SynthesisInput{
				InputSource: &texttospeechpb.SynthesisInput_Text{Text: text},
			},
			// Build the voice request, select the language code ("en-US") and the SSML
			// voice gender ("neutral").
			Voice: &texttospeechpb.VoiceSelectionParams{
				LanguageCode: lang,
				SsmlGender:   texttospeechpb.SsmlVoiceGender_NEUTRAL,
			},
			// Select the type of audio file you want returned.
			AudioConfig: &texttospeechpb.AudioConfig{
				AudioEncoding: texttospeechpb.AudioEncoding_MP3,
			},
		}

		resp, err := client.SynthesizeSpeech(ctx, &req)
		if err != nil {
			log.Fatal(err)
		}

		// The resp's AudioContent is binary.
		filename := "output.mp3"
		err = ioutil.WriteFile(filename, resp.AudioContent, 0644)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Audio content written to file: %v\n", filename)
	}

}
