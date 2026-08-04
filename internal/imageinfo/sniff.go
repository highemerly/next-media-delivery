package imageinfo

import (
	"bytes"
	"net/http"
	"strings"
)

// DetectContentType reports the media type of data based on its magic bytes,
// but only when that type is an image, video, or audio type. Anything else —
// including input whose type cannot be determined — yields "".
//
// It exists because object storage origins (Backblaze B2, S3 and friends)
// routinely serve media under a generic binary/octet-stream Content-Type.
// Judging such a response by its declared header alone would reject a
// perfectly valid image, so callers sniff the body before giving up.
func DetectContentType(data []byte) string {
	// ISO-BMFF first: net/http recognises only a narrow set of MP4 brands and
	// no HEIF derivative at all, so AVIF/HEIC would sniff as octet-stream.
	if ct := isobmffContentType(data); ct != "" {
		return ct
	}

	ct := http.DetectContentType(data)
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = ct[:i]
	}
	if strings.HasPrefix(ct, "image/") || strings.HasPrefix(ct, "video/") || strings.HasPrefix(ct, "audio/") {
		return ct
	}
	return ""
}

// isobmffContentType maps the major brand of an ISO base media file format
// container to its MIME type. Only the HEIF-family brands are handled here;
// MP4 and other video brands are left to http.DetectContentType.
func isobmffContentType(data []byte) string {
	if len(data) < 12 || !bytes.Equal(data[4:8], []byte("ftyp")) {
		return ""
	}
	switch string(data[8:12]) {
	case "avif", "avis":
		return "image/avif"
	case "heic", "heix", "hevc", "hevx", "heim", "heis", "hevm", "hevs":
		return "image/heic"
	case "mif1", "msf1":
		return "image/heif"
	}
	return ""
}
