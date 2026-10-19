package blob

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"os"
	"testing"
	"time"

	aacpcm "github.com/tphakala/go-aac/pcm"
	m4a "github.com/tphakala/go-m4a"
	"github.com/tphakala/go-m4a/aacm4a"
	"github.com/tphakala/go-opus/oggopus"
	wavpcm "github.com/tphakala/go-wav/pcm"
)

func tone(rate, channels int) []byte {
	pcm := []byte{}
	for i := range rate {
		s := int16(8000 * math.Sin(2*math.Pi*440*float64(i)/float64(rate)))
		for range channels {
			pcm = binary.LittleEndian.AppendUint16(pcm, uint16(s))
		}
	}
	return pcm
}

func TestChromesWebMIsReencoded(t *testing.T) {
	webm, err := os.ReadFile("testdata/chrome.webm")
	if err != nil {
		t.Fatal(err)
	}
	checkReencode(t, "audio/webm", webm)
}

func TestOggOpusIsReencoded(t *testing.T) {
	ogg := &bytes.Buffer{}
	if err := oggopus.EncodeInterleaved(ogg, oggopus.Config{SampleRate: 48000, Channels: 1}, tone(48000, 1)); err != nil {
		t.Fatal(err)
	}
	checkReencode(t, "audio/ogg", ogg.Bytes())
}

func TestStereoWAVAt44kIsReencoded(t *testing.T) {
	wav := &seekBuffer{}
	if err := wavpcm.EncodeInterleaved(wav, wavpcm.Config{SampleRate: 44100, BitDepth: 16, Channels: 2}, tone(44100, 2)); err != nil {
		t.Fatal(err)
	}
	checkReencode(t, "audio/wav", wav.b)
}

func TestAnM4AAt44kIsReencoded(t *testing.T) {
	in := &seekBuffer{}
	if err := aacm4a.EncodeInterleaved(in, aacpcm.Config{SampleRate: 44100, BitDepth: 16, Channels: 2, Bitrate: 192000}, tone(44100, 2)); err != nil {
		t.Fatal(err)
	}
	checkReencode(t, "audio/mp4", in.b)
}

func TestNotAudioIsRefused(t *testing.T) {
	for _, mimeType := range []string{"audio/webm", "audio/ogg", "audio/mpeg", "audio/wav", "audio/mp4"} {
		if _, err := ReencodeRecording(mimeType, []byte("not a recording")); err == nil {
			t.Errorf("%s: junk reencoded", mimeType)
		}
	}
}

func TestALowRateWAVLongerThanTheLimitIsRefused(t *testing.T) {
	wav := &seekBuffer{}
	if err := wavpcm.EncodeInterleaved(wav, wavpcm.Config{SampleRate: 1, BitDepth: 16, Channels: 1}, make([]byte, 2*100000)); err != nil {
		t.Fatal(err)
	}
	if _, err := ReencodeRecording("audio/wav", wav.b); err == nil {
		t.Fatal("100,000 samples at 1 Hz were resampled")
	}
}

func checkReencode(t *testing.T, mimeType string, content []byte) {
	t.Helper()
	start := time.Now()
	out, err := ReencodeRecording(mimeType, content)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s: %d bytes to %d in %v", mimeType, len(content), len(out), time.Since(start))
	dec, info, err := aacm4a.NewDecoder(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if info.Codec != m4a.CodecAACLC || info.SampleRate != 48000 || info.Channels != 1 {
		t.Fatalf("not mono 48 kHz AAC-LC: %+v", info)
	}
	if d := info.Duration - time.Second; d < -100*time.Millisecond || d > 100*time.Millisecond {
		t.Fatalf("lasts %v, recorded a second", info.Duration)
	}
	pcm, err := io.ReadAll(dec)
	if err != nil {
		t.Fatal(err)
	}
	pcm = pcm[2*info.EncoderDelay : 2*(info.EncoderDelay+int64(info.Duration.Seconds()*48000))]
	var sum float64
	crossings := 0
	last := 0.0
	for i := 0; i+1 < len(pcm); i += 2 {
		s := float64(int16(binary.LittleEndian.Uint16(pcm[i:])))
		sum += s * s
		if (s >= 0) != (last >= 0) {
			crossings++
		}
		last = s
	}
	if rms := math.Sqrt(sum / float64(len(pcm)/2)); rms < 1000 {
		t.Fatalf("decodes to near silence: rms %.0f", rms)
	}
	if hz := float64(crossings) / 2 / info.Duration.Seconds(); hz < 420 || hz > 460 {
		t.Fatalf("a 440 Hz tone comes out at %.0f Hz", hz)
	}
}
