// node name constants to avoid circular import issues between block modules

export const CODE_BLOCK_TITLE_NAME = "codeBlockTitle"
export const CODE_BLOCK_NAME = "codeBlock"
export const TITLED_CODE_BLOCK_NAME = "titledCodeBlock"
export const METRIC_BLOCK_NAME = "metricBlock"
export const LOG_BLOCK_NAME = "logBlock"
export const BLOCK_GRID_NAME = "blockGrid"
// the legacy name of the block grid. Documents may still hold it, and the
// editor turns one into a block grid on its first local edit.
export const LEGACY_METRIC_GRID_NAME = "metricGrid"
// the group of the blocks a grid lays out side by side
export const GRID_ITEM_GROUP = "gridItem"
// the group of the blocks only the document root takes, such as grids
export const ROOT_ONLY_BLOCK_GROUP = "rootOnlyBlock"
export const IMAGE_BLOCK_NAME = "imageBlock"
export const FILE_BLOCK_NAME = "fileBlock"
export const CALLOUT_BLOCK_NAME = "calloutBlock"

export const MERMAID_BLOCK_NAME = "mermaidBlock"

export const FIGMA_BLOCK_NAME = "figmaBlock"

export const SPLIT_DOCUMENTATION_NAME = "splitDocumentation"
export const SPLIT_DOCUMENTATION_LEFT_SIDE_NAME = "splitDocumentationLeftSide"
export const SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME = "splitDocumentationRightSide"
export const SPLIT_DOCUMENTATION_PARAMETER_LIST_NAME =
	"splitDocumentationParameterList"
export const SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_NAME =
	"splitDocumentationParameterListItem"
export const SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_HEADER_NAME =
	"splitDocumentationParameterListItemHeader"
export const SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_HEADER_TITLE_NAME =
	"splitDocumentationParameterListItemHeaderTitle"
export const SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_HEADER_TYPE_NAME =
	"splitDocumentationParameterListItemHeaderType"
export const SPLIT_DOCUMENTATION_PARAMETER_LIST_HEADER_NAME =
	"splitDocumentationParameterListHeader"
