package slackhandler

import (
	"github.com/oxynote/oxynote/server/core/pkg/ptrutil"
	"github.com/slack-go/slack"
)

// _welcomeMessageTemplate is a template for the welcome message sent to users.
const _welcomeMessageTemplate = `Hey <@%s> 👋! Oxynote’s Slack integration 
allows you to easily share context, save important information, and create 
new documents. See our help documentation for more information.`

// connectOrganizationMessage creates a Slack message option with a welcome text and a button to connect Oxynote.
func connectOrganizationMessage(text, url string) slack.MsgOption {
	return slack.MsgOptionBlocks(
		&slack.SectionBlock{
			Type: slack.MBTSection,
			Text: &slack.TextBlockObject{
				Type: slack.MarkdownType,
				Text: text,
			},
		},
		&slack.ActionBlock{
			Type:    slack.MBTAction,
			BlockID: "connect_organization_block",
			Elements: &slack.BlockElements{
				ElementSet: []slack.BlockElement{
					&slack.ButtonBlockElement{
						ActionID: "connect_organization_element",
						Type:     slack.METButton,
						Style:    slack.StylePrimary,
						Text: &slack.TextBlockObject{
							Type: slack.MarkdownType,
							Text: "Connect Oxynote",
						},
						URL: url,
					},
				},
			},
		},
	)
}

// connectOrganizationModal creates a Slack modal view request for connecting a organization.
func connectOrganizationModal(url string) slack.ModalViewRequest {
	return slack.ModalViewRequest{
		Type: slack.VTModal,
		Title: &slack.TextBlockObject{
			Type: slack.PlainTextType,
			Text: "Connect Oxynote",
		},
		Blocks: slack.Blocks{
			BlockSet: []slack.Block{
				&slack.SectionBlock{
					Type: slack.MBTSection,
					Text: &slack.TextBlockObject{
						Type: slack.MarkdownType,
						Text: "Connect Oxynote to this Slack organization to create Oxynote notes from within Slack.",
					},
				},
				&slack.SectionBlock{
					Type: slack.MBTSection,
					Text: &slack.TextBlockObject{
						Type: slack.MarkdownType,
						Text: "Would you like to connect?",
					},
				},
				&slack.ActionBlock{
					Type:    slack.MBTAction,
					BlockID: "connect_organization_action_block",
					Elements: &slack.BlockElements{
						ElementSet: []slack.BlockElement{
							&slack.ButtonBlockElement{
								Type:  slack.METButton,
								Style: slack.StylePrimary,
								URL:   url,
								Text: &slack.TextBlockObject{
									Type: slack.PlainTextType,
									Text: "Connect Oxynote",
								},
							},
						},
					},
				},
			},
		},
		Close: &slack.TextBlockObject{
			Type: slack.PlainTextType,
			Text: "Dismiss",
		},
	}
}

// initialMessageSavingModal creates a Slack modal view request for saving a message.
func initialMessageSavingModal(text string) slack.ModalViewRequest {
	return slack.ModalViewRequest{
		Type: slack.VTModal,
		Title: &slack.TextBlockObject{
			Type:  slack.PlainTextType,
			Text:  "Create a new message",
			Emoji: ptrutil.New(false),
		},
		Blocks: slack.Blocks{
			BlockSet: []slack.Block{
				&slack.InputBlock{
					Type:    slack.MBTInput,
					BlockID: "message_input_block",
					Label: &slack.TextBlockObject{
						Type: slack.PlainTextType,
						Text: "Text",
					},
					Element: &slack.PlainTextInputBlockElement{
						Type:         slack.METPlainTextInput,
						ActionID:     "message_input_block_action",
						Multiline:    true,
						InitialValue: text,
						Placeholder: &slack.TextBlockObject{
							Type: slack.PlainTextType,
							Text: "Enter the message you want to save",
						},
					},
				},
			},
		},
		Submit: &slack.TextBlockObject{
			Type:  slack.PlainTextType,
			Text:  "Submit",
			Emoji: ptrutil.New(false),
		},
	}
}
