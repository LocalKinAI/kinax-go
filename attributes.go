package kinax

// Standard AXUIElement attribute names. These are stable strings from
// <HIServices/AXAttributeConstants.h>. Use them rather than string
// literals so typos fail at compile time.
const (
	AttrRole             = "AXRole"
	AttrSubrole          = "AXSubrole"
	AttrRoleDescription  = "AXRoleDescription"
	AttrTitle            = "AXTitle"
	AttrDescription      = "AXDescription"
	AttrHelp             = "AXHelp"
	AttrValue            = "AXValue"
	AttrValueDescription = "AXValueDescription"
	AttrPlaceholder      = "AXPlaceholderValue"
	AttrIdentifier       = "AXIdentifier"
	AttrEnabled          = "AXEnabled"
	AttrFocused          = "AXFocused"
	AttrSelected         = "AXSelected"
	AttrVisible          = "AXVisible"
	AttrExpanded         = "AXExpanded"
	AttrMain             = "AXMain" // primary window
	AttrMinimized        = "AXMinimized"
	AttrFullscreen       = "AXFullscreen"

	// Geometry
	AttrPosition = "AXPosition" // CGPoint
	AttrSize     = "AXSize"     // CGSize
	AttrFrame    = "AXFrame"    // CGRect (not all elements expose this)

	// Tree navigation
	AttrParent          = "AXParent"
	AttrChildren        = "AXChildren"
	AttrChildrenInOrder = "AXChildrenInNavigationOrder"
	AttrWindows         = "AXWindows"
	AttrMainWindow      = "AXMainWindow"
	AttrFocusedWindow   = "AXFocusedWindow"
	AttrFocusedElement  = "AXFocusedUIElement"
	AttrMenuBar         = "AXMenuBar"
	AttrTopLevelElement = "AXTopLevelUIElement"

	// Containers
	AttrRows               = "AXRows"
	AttrColumns            = "AXColumns"
	AttrCell               = "AXCell"
	AttrTabs               = "AXTabs"
	AttrSelectedText       = "AXSelectedText"
	AttrNumberOfCharacters = "AXNumberOfCharacters"
)

// Standard AX role values (stable constants from AXRoleConstants.h).
// Use with [Element.Role] comparisons.
const (
	RoleApplication = "AXApplication"
	RoleWindow      = "AXWindow"
	RoleButton      = "AXButton"
	RoleCheckBox    = "AXCheckBox"
	RoleRadioButton = "AXRadioButton"
	RoleTextField   = "AXTextField"
	RoleTextArea    = "AXTextArea"
	RoleStaticText  = "AXStaticText"
	RolePopUpButton = "AXPopUpButton"
	RoleMenu        = "AXMenu"
	RoleMenuItem    = "AXMenuItem"
	RoleMenuBar     = "AXMenuBar"
	RoleMenuBarItem = "AXMenuBarItem"
	RoleGroup       = "AXGroup"
	RoleList        = "AXList"
	RoleTable       = "AXTable"
	RoleRow         = "AXRow"
	RoleCell        = "AXCell"
	RoleColumn      = "AXColumn"
	RoleScrollArea  = "AXScrollArea"
	RoleScrollBar   = "AXScrollBar"
	RoleSlider      = "AXSlider"
	RoleToolbar     = "AXToolbar"
	RoleImage       = "AXImage"
	RoleLink        = "AXLink"
	RoleTabGroup    = "AXTabGroup"
	RoleSheet       = "AXSheet"
	RoleSplitGroup  = "AXSplitGroup"
	RoleOutline     = "AXOutline"
	RoleWebArea     = "AXWebArea"
)

// Standard AX actions (stable constants from AXActionConstants.h).
const (
	ActionPress           = "AXPress"
	ActionIncrement       = "AXIncrement"
	ActionDecrement       = "AXDecrement"
	ActionConfirm         = "AXConfirm"
	ActionCancel          = "AXCancel"
	ActionShowMenu        = "AXShowMenu"
	ActionRaise           = "AXRaise"
	ActionShowAlternateUI = "AXShowAlternateUI"
	ActionShowDefaultUI   = "AXShowDefaultUI"
	ActionPick            = "AXPick"
)
