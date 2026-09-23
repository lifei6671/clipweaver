package media

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strconv"

	"github.com/lifei6671/clipweaver/internal/domain"
)

var ErrFFmpegFailed = errors.New("FFMPEG_FAILED")

const maxFFmpegStderr = 16 * 1024

// FFmpegError exposes bounded diagnostics to server logging while keeping Error stable for clients.
type FFmpegError struct {
	Phase  string
	Stderr string
	Cause  error
}

func (e *FFmpegError) Error() string   { return ErrFFmpegFailed.Error() }
func (e *FFmpegError) Unwrap() []error { return []error{ErrFFmpegFailed, e.Cause} }

// boundedStderr retains only the last bytes, where FFmpeg usually reports the failure.
type boundedStderr struct{ bytes.Buffer }

func (b *boundedStderr) Write(p []byte) (int, error) {
	n := len(p)
	if n >= maxFFmpegStderr {
		b.Buffer.Reset()
		_, _ = b.Buffer.Write(p[n-maxFFmpegStderr:])
		return n, nil
	}
	if excess := b.Len() + n - maxFFmpegStderr; excess > 0 {
		b.Buffer.Next(excess)
	}
	_, _ = b.Buffer.Write(p)
	return n, nil
}

func runFFmpeg(ctx context.Context, binary string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, binary, args...)
	var stderr boundedStderr
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stderr.String(), err
}

// secondsUS renders exact integer microseconds without floating point conversion.
func secondsUS(us domain.DurationUS) string {
	whole := int64(us) / 1_000_000
	fraction := int64(us) % 1_000_000
	if fraction == 0 {
		return strconv.FormatInt(whole, 10)
	}
	var digits [6]byte
	for i := 5; i >= 0; i-- {
		digits[i] = byte('0' + fraction%10)
		fraction /= 10
	}
	end := len(digits)
	for end > 0 && digits[end-1] == '0' {
		end--
	}
	return strconv.FormatInt(whole, 10) + "." + string(digits[:end])
}

func clipArgs(input, output string, clip domain.PlannedClip) []string {
	filter := "trim=start=" + secondsUS(clip.StartUS) + ":end=" + secondsUS(clip.StartUS+clip.DurationUS) +
		",setpts=PTS-STARTPTS,scale=1080:1920:force_original_aspect_ratio=decrease," +
		"pad=1080:1920:(ow-iw)/2:(oh-ih)/2:black,fps=30,setsar=1,format=yuv420p"
	return []string{"-hide_banner", "-nostdin", "-v", "error", "-y", "-i", input,
		"-map", "0:v:0", "-vf", filter, "-an", "-c:v", "libx264", "-pix_fmt", "yuv420p", output}
}

func concatArgs(manifest, output string) []string {
	return []string{"-hide_banner", "-nostdin", "-v", "error", "-y", "-f", "concat", "-safe", "1", "-i", manifest,
		"-map", "0:v:0", "-an", "-c", "copy", output}
}

func narrationNormalizeArgs(input, output string) []string {
	// Rebuild timestamps from decoded samples without adding or repeating audio.
	return []string{"-hide_banner", "-nostdin", "-v", "error", "-y", "-i", input,
		"-map", "0:a:0", "-vn", "-af", "asetpts=N/SR/TB", "-c:a", "pcm_s16le", "-f", "wav", output}
}

func videoFinalizeArgs(silent, output string, target domain.DurationUS) []string {
	return []string{"-hide_banner", "-nostdin", "-v", "error", "-y", "-i", silent,
		"-map", "0:v:0", "-vf", "tpad=stop=-1:stop_mode=clone", "-t", secondsUS(target),
		"-an", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-r", "30", "-f", "mp4", output}
}

func muxArgs(video, narration, output string, target domain.DurationUS) []string {
	return []string{"-hide_banner", "-nostdin", "-v", "error", "-y", "-i", video, "-i", narration,
		"-map", "0:v:0", "-map", "1:a:0", "-t", secondsUS(target),
		"-c:v", "copy", "-c:a", "aac", "-movflags", "+faststart", "-f", "mp4", output}
}
