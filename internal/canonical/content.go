package canonical

// ContentPartType identifies a canonical message content block.
type ContentPartType string

const (
	ContentPartText     ContentPartType = "text"
	ContentPartImageURL ContentPartType = "image_url"
	ContentPartVideoURL ContentPartType = "video_url"
)

type ImageURL struct {
	URL    string
	Detail string
}

type VideoURL struct {
	URL string
}

type ContentPart struct {
	Type     ContentPartType
	Text     string
	ImageURL *ImageURL
	VideoURL *VideoURL
}

func TextContent(text string) []ContentPart {
	return []ContentPart{{Type: ContentPartText, Text: text}}
}
