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

	// AttrName is the file name a file block shows.
	AttrName = "name"

	// AttrSize is the size in bytes of a file block's file.
	AttrSize = "size"

	// AttrContentType is the media type of a file block's file.
	AttrContentType = "contentType"

	// AttrInversed indicates that a split documentation macro renders its
	// sides the other way round.
	AttrInversed = "inversed"

	// AttrChecked indicates that a task item is done.
	AttrChecked = "checked"

	// AttrStart is the number an ordered list counts from.
	AttrStart = "start"

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

	// AttrUnitType is the unit a metric block's values carry.
	AttrUnitType = "unitType"

	// AttrSimulationPreset names the generated series a metric block
	// draws while AttrSimulationActive is set. It is unset on a block
	// nobody has simulated. Once set, it stays after the simulation ends.
	AttrSimulationPreset = "simulationPreset"

	// AttrSimulationActive is true while a metric block draws its
	// AttrSimulationPreset because its data source has no real data yet.
	// Only the editor and core use it. Tools and document readers never
	// see it.
	AttrSimulationActive = "simulationActive"
)

// MarkComment is the inline mark type anchoring a comment to a range of
// text.
const MarkComment = "comment"
