package blob

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"math/bits"
	"strings"

	"github.com/hajimehoshi/go-mp3"
	aacpcm "github.com/tphakala/go-aac/pcm"
	m4a "github.com/tphakala/go-m4a"
	"github.com/tphakala/go-m4a/aacm4a"
	"github.com/tphakala/go-m4a/opusm4a"
	"github.com/tphakala/go-opus/oggopus"
	"github.com/tphakala/go-opus/opus"
	wavpcm "github.com/tphakala/go-wav/pcm"
)

var audioExtensions = map[string]string{
	"audio/webm":  "webm",
	"video/webm":  "webm",
	"audio/mp4":   "m4a",
	"video/mp4":   "m4a",
	"audio/x-m4a": "m4a",
	"audio/mpeg":  "mp3",
	"audio/ogg":   "ogg",
	"audio/wav":   "wav",
}

const (
	recordingRate       = 48000
	recordingBitrate    = 64000
	maxRecordingSeconds = 600
	maxDecodedPCM       = recordingRate * 2 * 2 * maxRecordingSeconds
	resampleTaps        = 16
)

func AudioType(declared string) (string, string, bool) {
	base, _, _ := strings.Cut(declared, ";")
	base = strings.TrimSpace(strings.ToLower(base))
	ext, ok := audioExtensions[base]
	if !ok {
		return "", "", false
	}
	if strings.HasPrefix(base, "video/") {
		base = "audio/" + strings.TrimPrefix(base, "video/")
	}
	return base, ext, true
}

func ReencodeRecording(mimeType string, content []byte) ([]byte, error) {
	pcm, rate, channels, err := decodeAudio(mimeType, content)
	if err != nil {
		return nil, err
	}
	if channels < 1 || rate < 1 {
		return nil, fmt.Errorf("%s declares %d channels at %d Hz", mimeType, channels, rate)
	}
	mono := toMono(pcm, channels)
	if float64(len(mono))/float64(rate) > maxRecordingSeconds {
		return nil, fmt.Errorf("the recording lasts more than %d seconds", maxRecordingSeconds)
	}
	if rate != recordingRate {
		mono = resample(mono, rate)
	}
	out := &seekBuffer{}
	cfg := aacpcm.Config{SampleRate: recordingRate, BitDepth: 16, Channels: 1, Bitrate: recordingBitrate}
	if err := aacm4a.EncodeInterleaved(out, cfg, toPCM(mono)); err != nil {
		return nil, err
	}
	return out.b, nil
}

func decodeAudio(mimeType string, content []byte) ([]byte, int, int, error) {
	switch mimeType {
	case "audio/webm":
		pcm, channels, err := decodeWebM(content)
		return pcm, recordingRate, channels, err
	case "audio/ogg":
		pcm, info, err := oggopus.DecodeInterleavedLimit(bytes.NewReader(content), maxDecodedPCM)
		return pcm, int(info.OutputSampleRate), info.Channels, err
	case "audio/mpeg":
		dec, err := mp3.NewDecoder(bytes.NewReader(content))
		if err != nil {
			return nil, 0, 0, err
		}
		pcm, err := readLimited(dec)
		return pcm, dec.SampleRate(), 2, err
	case "audio/wav":
		pcm, info, err := wavpcm.DecodeInterleavedLimit(bytes.NewReader(content), maxDecodedPCM, wavpcm.WithConvertTo(16))
		return pcm, info.SampleRate, info.Channels, err
	case "audio/mp4", "audio/x-m4a":
		return decodeMP4(content)
	}
	return nil, 0, 0, fmt.Errorf("%s is not a recording type", mimeType)
}

func decodeMP4(content []byte) ([]byte, int, int, error) {
	rd, err := m4a.NewReader(bytes.NewReader(content))
	if err != nil {
		return nil, 0, 0, err
	}
	if rd.Info().Codec == m4a.CodecOpus {
		pcm, info, err := opusm4a.DecodeInterleavedLimit(bytes.NewReader(content), maxDecodedPCM)
		return pcm, recordingRate, info.Channels, err
	}
	dec, info, err := aacm4a.NewDecoder(bytes.NewReader(content))
	if err != nil {
		return nil, 0, 0, err
	}
	pcm, err := readLimited(dec)
	if err != nil {
		return nil, 0, 0, err
	}
	frame := 2 * info.Channels
	start := min(int(info.EncoderDelay)*frame, len(pcm))
	end := min(start+int(math.Ceil(info.Duration.Seconds()*float64(info.SampleRate)))*frame, len(pcm))
	return pcm[start:end], info.SampleRate, info.Channels, nil
}

func readLimited(r io.Reader) ([]byte, error) {
	pcm, err := io.ReadAll(io.LimitReader(r, maxDecodedPCM+1))
	if err != nil {
		return nil, err
	}
	if len(pcm) > maxDecodedPCM {
		return nil, errors.New("the recording is too long")
	}
	return pcm, nil
}

func toMono(pcm []byte, channels int) []float64 {
	frames := len(pcm) / (2 * channels)
	out := make([]float64, frames)
	for i := range out {
		var sum float64
		for c := range channels {
			sum += float64(int16(binary.LittleEndian.Uint16(pcm[2*(i*channels+c):])))
		}
		out[i] = sum / float64(channels)
	}
	return out
}

func resample(in []float64, from int) []float64 {
	ratio := float64(recordingRate) / float64(from)
	cutoff := min(1, ratio)
	out := make([]float64, int(float64(len(in))*ratio))
	for i := range out {
		t := float64(i) / ratio
		center := int(math.Floor(t))
		var sum float64
		for j := max(0, center-resampleTaps+1); j <= min(len(in)-1, center+resampleTaps); j++ {
			x := t - float64(j)
			window := 0.5 + 0.5*math.Cos(math.Pi*x/resampleTaps)
			sum += in[j] * cutoff * sinc(cutoff*x) * window
		}
		out[i] = sum
	}
	return out
}

func sinc(x float64) float64 {
	if x == 0 {
		return 1
	}
	return math.Sin(math.Pi*x) / (math.Pi * x)
}

func toPCM(samples []float64) []byte {
	out := []byte{}
	for _, s := range samples {
		out = binary.LittleEndian.AppendUint16(out, uint16(int16(math.Round(max(math.MinInt16, min(math.MaxInt16, s))))))
	}
	return out
}

const (
	ebmlSegment      = 0x18538067
	ebmlTracks       = 0x1654AE6B
	ebmlTrackEntry   = 0xAE
	ebmlTrackNumber  = 0xD7
	ebmlCodecID      = 0x86
	ebmlCodecPrivate = 0x63A2
	ebmlCluster      = 0x1F43B675
	ebmlBlockGroup   = 0xA0
	ebmlBlock        = 0xA1
	ebmlSimpleBlock  = 0xA3
)

type webmTrack struct {
	number  uint64
	codec   string
	private []byte
}

type webmBlock struct {
	track uint64
	data  []byte
}

func decodeWebM(b []byte) ([]byte, int, error) {
	tracks := []*webmTrack{}
	blocks := []webmBlock{}
	for len(b) > 0 {
		id, n, ok := ebmlVint(b, true)
		if !ok {
			return nil, 0, errors.New("webm: bad element id")
		}
		size, m, ok := ebmlVint(b[n:], false)
		if !ok {
			return nil, 0, errors.New("webm: bad element size")
		}
		b = b[n+m:]
		switch id {
		case ebmlSegment, ebmlTracks, ebmlCluster, ebmlBlockGroup:
			continue
		case ebmlTrackEntry:
			tracks = append(tracks, &webmTrack{})
			continue
		}
		if size == 1<<(7*m)-1 || size > uint64(len(b)) {
			return nil, 0, fmt.Errorf("webm: element %x runs past the file", id)
		}
		body := b[:size]
		b = b[size:]
		if len(tracks) == 0 && (id == ebmlTrackNumber || id == ebmlCodecID || id == ebmlCodecPrivate) {
			return nil, 0, errors.New("webm: track field outside a track entry")
		}
		switch id {
		case ebmlTrackNumber:
			for _, c := range body {
				tracks[len(tracks)-1].number = tracks[len(tracks)-1].number<<8 | uint64(c)
			}
		case ebmlCodecID:
			tracks[len(tracks)-1].codec = string(body)
		case ebmlCodecPrivate:
			tracks[len(tracks)-1].private = body
		case ebmlBlock, ebmlSimpleBlock:
			track, k, ok := ebmlVint(body, false)
			if !ok || len(body) < k+3 {
				return nil, 0, errors.New("webm: bad block header")
			}
			if body[k+2]&0x06 != 0 {
				return nil, 0, errors.New("webm: laced blocks are not supported")
			}
			blocks = append(blocks, webmBlock{track: track, data: body[k+3:]})
		}
	}
	var track *webmTrack
	for _, t := range tracks {
		if t.codec == "A_OPUS" {
			track = t
			break
		}
	}
	if track == nil {
		return nil, 0, errors.New("webm: no opus track")
	}
	head := track.private
	if len(head) < 19 || string(head[:8]) != "OpusHead" || head[18] != 0 {
		return nil, 0, errors.New("webm: the opus track has no mono or stereo OpusHead")
	}
	channels := int(head[9])
	preSkip := int(binary.LittleEndian.Uint16(head[10:12]))
	dec, err := opus.NewDecoder(recordingRate, channels)
	if err != nil {
		return nil, 0, err
	}
	frame := make([]int16, recordingRate*120/1000*channels)
	pcm := []byte{}
	for _, block := range blocks {
		if block.track != track.number {
			continue
		}
		n, err := dec.Decode(block.data, frame)
		if err != nil {
			return nil, 0, err
		}
		for _, s := range frame[:n*channels] {
			pcm = binary.LittleEndian.AppendUint16(pcm, uint16(s))
		}
		if len(pcm) > maxDecodedPCM {
			return nil, 0, errors.New("webm: the recording is too long")
		}
	}
	skip := min(preSkip*channels*2, len(pcm))
	return pcm[skip:], channels, nil
}

func ebmlVint(b []byte, keepMarker bool) (uint64, int, bool) {
	if len(b) == 0 || b[0] == 0 {
		return 0, 0, false
	}
	n := bits.LeadingZeros8(b[0]) + 1
	if len(b) < n {
		return 0, 0, false
	}
	v := uint64(b[0])
	if !keepMarker {
		v &= 0xff >> n
	}
	for _, c := range b[1:n] {
		v = v<<8 | uint64(c)
	}
	return v, n, true
}

type seekBuffer struct {
	b   []byte
	off int
}

func (s *seekBuffer) Write(p []byte) (int, error) {
	if grow := s.off + len(p) - len(s.b); grow > 0 {
		s.b = append(s.b, make([]byte, grow)...)
	}
	copy(s.b[s.off:], p)
	s.off += len(p)
	return len(p), nil
}

func (s *seekBuffer) Seek(offset int64, whence int) (int64, error) {
	base := 0
	switch whence {
	case io.SeekCurrent:
		base = s.off
	case io.SeekEnd:
		base = len(s.b)
	}
	off := base + int(offset)
	if off < 0 {
		return 0, errors.New("seek before the start")
	}
	s.off = off
	return int64(off), nil
}
