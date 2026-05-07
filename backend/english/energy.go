package english

import (
	"fmt"
	"io"
	"math"
	"os"

	"github.com/hajimehoshi/go-mp3"
)

// silenceThresholdDB: 三段 RMS 全部低于此 dB 视为静音占位。
// 实测 Akamai CDN VOA 静音 mp3 RMS ≈ -90dB, 正常人声节目 ≈ -25 至 -15dB,
// -50dB 阈值能拉开静音占位与白噪 / 极弱信号的距离。
const silenceThresholdDB = -50.0

// maxAudioBytes: Groq Whisper API 单文件上限 25MB; mac 端 Transcriber 上传超此值会 413 / 失败。
// fetcher 下载完后 stat 文件大小超此值即标 failed (跟 silence 同 validation chain)。
//
// duration cap (`maxDurationMs` in fetcher.go) 按 128kbps 估算, 但实际 bitrate 不可控
// (Sciam Science Quickly 用 192kbps, 24min ≈ 33MB 超 25MB), 所以 size cap 是 duration
// cap 之外的硬保险。
const maxAudioBytes int64 = 25 * 1024 * 1024

// ValidateAudioSize 检查 mp3 文件大小是否在 Groq Whisper 上限 (25MB) 内。
// 超出返 error, 让 fetcher 标 segments_status=failed 跳过 ASR 阶段。
//
// 跟 ValidateAudioEnergy 平级的 validation gate, fetcher 下载后 chain 调两个。
// stat 失败时 fail-close (返 error 让 fetcher 处理), 防 file IO 异常掩盖大文件入库。
func ValidateAudioSize(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat mp3: %w", err)
	}
	if info.Size() > maxAudioBytes {
		mb := info.Size() / (1024 * 1024)
		return fmt.Errorf("file too large for Whisper (%dMB > %dMB)", mb, maxAudioBytes/(1024*1024))
	}
	return nil
}

// ValidateAudioEnergy 探测 mp3 是否可能是静音占位 (issue: VOA Akamai CDN 全 -90dB 占位坑)。
// 头 / 中 / 尾各取 ~5s PCM RMS, 三段全部 < -50dB 视为 silent → 返 error。
// decode 失败时 fail-open 返 nil (合法 mp3 偶发解码失败时不要误杀)。
func ValidateAudioEnergy(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open mp3: %w", err)
	}
	defer f.Close()

	d, err := mp3.NewDecoder(f)
	if err != nil {
		return nil // fail-open
	}
	sampleRate := d.SampleRate()
	if sampleRate <= 0 {
		return nil
	}
	// go-mp3 always outputs 16-bit signed LE stereo PCM = 4 bytes / sample frame
	const bytesPerFrame int64 = 4
	segLen := int64(sampleRate) * 5 * bytesPerFrame
	total := d.Length()

	if total < segLen*3 {
		return rmsCheckShort(d, total)
	}

	db1, err := readSegRMS(d, 0, segLen)
	if err != nil {
		return nil
	}
	midStart := alignFrame(total/2, bytesPerFrame)
	db2, err := readSegRMS(d, midStart, segLen)
	if err != nil {
		return nil
	}
	tailStart := alignFrame(total-segLen, bytesPerFrame)
	db3, err := readSegRMS(d, tailStart, segLen)
	if err != nil {
		return nil
	}

	if db1 < silenceThresholdDB && db2 < silenceThresholdDB && db3 < silenceThresholdDB {
		mean := (db1 + db2 + db3) / 3
		return fmt.Errorf("silent audio (mean RMS %.1fdB across head/mid/tail)", mean)
	}
	return nil
}

func alignFrame(off, frame int64) int64 {
	if off < 0 {
		return 0
	}
	return (off / frame) * frame
}

func readSegRMS(d *mp3.Decoder, start, length int64) (float64, error) {
	if _, err := d.Seek(start, io.SeekStart); err != nil {
		return 0, err
	}
	buf := make([]byte, length)
	n, err := io.ReadFull(d, buf)
	if err != nil && err != io.ErrUnexpectedEOF {
		return 0, err
	}
	return rmsDB(buf[:n]), nil
}

func rmsCheckShort(d *mp3.Decoder, length int64) error {
	if length <= 0 {
		return nil
	}
	if _, err := d.Seek(0, io.SeekStart); err != nil {
		return nil
	}
	buf := make([]byte, length)
	n, err := io.ReadFull(d, buf)
	if err != nil && err != io.ErrUnexpectedEOF {
		return nil
	}
	db := rmsDB(buf[:n])
	if db < silenceThresholdDB {
		return fmt.Errorf("silent audio (RMS %.1fdB short file)", db)
	}
	return nil
}

func rmsDB(pcm []byte) float64 {
	const minDB = -100.0
	if len(pcm) < 2 {
		return minDB
	}
	var sumSq float64
	var count int64
	for i := 0; i+1 < len(pcm); i += 2 {
		v := int16(uint16(pcm[i]) | uint16(pcm[i+1])<<8)
		f := float64(v) / 32768.0
		sumSq += f * f
		count++
	}
	if count == 0 {
		return minDB
	}
	rms := math.Sqrt(sumSq / float64(count))
	if rms <= 0 {
		return minDB
	}
	return 20 * math.Log10(rms)
}
