package media

type Kind string

const (
	KindAvatar Kind = "avatar"
	KindFeed   Kind = "feed"
	KindChat   Kind = "chat"
)

type kindRule struct {
	prefix  string
	maxSize int64
	public  bool
}

var kindRules = map[Kind]kindRule{
	KindAvatar: {prefix: "avatars", maxSize: 5 << 20, public: true},
	KindFeed:   {prefix: "feed", maxSize: 10 << 20, public: true},
	KindChat:   {prefix: "chat", maxSize: 10 << 20, public: false},
}

var extensions = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
}
