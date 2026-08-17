package document

// Attribute keys carried by ProseMirror nodes. They are the wire
// vocabulary the TipTap schema registers, so they are declared here once
// and consumed by everything that reads or writes a node's attrs.
const (
	// AttrUID is the stable per-node identifier every block carries.
	AttrUID = "uid"

	// AttrCommentID is the identifier a comment mark points at.
	AttrCommentID = "nodeCommentId"

	// AttrLevel is the heading level.
	AttrLevel = "level"

	// AttrIcon is the callout icon.
	AttrIcon = "icon"

	// AttrLanguage is the code block language.
	AttrLanguage = "language"

	// AttrSrc is the source URL of an image or an embed.
	AttrSrc = "src"

	// AttrAlt is the alternative text of an image.
	AttrAlt = "alt"

	// AttrTitle is the title of an image or a titled code block.
	AttrTitle = "title"

	// AttrWidth is the rendered width of an image or an embed.
	AttrWidth = "width"

	// AttrHeight is the rendered height of an embed.
	AttrHeight = "height"

	// AttrInversed indicates that a split documentation macro renders its
	// sides the other way round.
	AttrInversed = "inversed"

	// AttrChecked indicates that a task item is done.
	AttrChecked = "checked"
)

// MarkComment is the inline mark type anchoring a comment to a range of
// text.
const MarkComment = "comment"
