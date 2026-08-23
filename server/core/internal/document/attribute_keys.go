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

	// AttrDataSourceID is the data source a metric block queries.
	AttrDataSourceID = "dataSourceId"

	// AttrVisualizationType is the chart a metric block renders.
	AttrVisualizationType = "visualizationType"

	// AttrQueries holds a metric block's query rows.
	AttrQueries = "queries"

	// AttrTimeRange is the preset window a metric block renders.
	AttrTimeRange = "timeRange"

	// AttrRefreshInterval is how often a metric block re-queries.
	AttrRefreshInterval = "refreshInterval"

	// AttrThresholds holds a metric block's threshold rows.
	AttrThresholds = "thresholds"

	// AttrBaseThresholdColor is a gauge's base colour.
	AttrBaseThresholdColor = "baseThresholdColor"

	// AttrDecimals is the number of decimals a metric block shows.
	AttrDecimals = "decimals"

	// AttrUnitType is the unit a metric block's values carry.
	AttrUnitType = "unitType"

	// AttrUnitCustom is the unit label when AttrUnitType is custom.
	AttrUnitCustom = "unitCustom"

	// AttrAxisBoundsMin is a metric block's fixed lower axis bound.
	AttrAxisBoundsMin = "axisBoundsMin"

	// AttrAxisBoundsMax is a metric block's fixed upper axis bound.
	AttrAxisBoundsMax = "axisBoundsMax"
)

// MarkComment is the inline mark type anchoring a comment to a range of
// text.
const MarkComment = "comment"
