// Package botapi contains the small subset of official Telegram Bot API types
// needed by the adapter. These types never cross the adapter boundary.
package botapi

type User struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name,omitempty"`
	Username  string `json:"username,omitempty"`
}

type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

type FileRef struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	FileSize     int64  `json:"file_size,omitempty"`
}

type PhotoSize struct {
	FileRef
	Width  int `json:"width"`
	Height int `json:"height"`
}

type Audio struct {
	FileRef
	FileName string `json:"file_name,omitempty"`
	MIMEType string `json:"mime_type,omitempty"`
}

type Video struct {
	FileRef
	FileName string `json:"file_name,omitempty"`
	MIMEType string `json:"mime_type,omitempty"`
}

type Voice struct {
	FileRef
	MIMEType string `json:"mime_type,omitempty"`
}

type Document struct {
	FileRef
	FileName string `json:"file_name,omitempty"`
	MIMEType string `json:"mime_type,omitempty"`
}

type Message struct {
	MessageID      int64       `json:"message_id"`
	From           *User       `json:"from,omitempty"`
	Chat           Chat        `json:"chat"`
	Date           int64       `json:"date"`
	EditDate       int64       `json:"edit_date,omitempty"`
	Text           string      `json:"text,omitempty"`
	Caption        string      `json:"caption,omitempty"`
	Photo          []PhotoSize `json:"photo,omitempty"`
	Video          *Video      `json:"video,omitempty"`
	Audio          *Audio      `json:"audio,omitempty"`
	Voice          *Voice      `json:"voice,omitempty"`
	Document       *Document   `json:"document,omitempty"`
	ReplyToMessage *Message    `json:"reply_to_message,omitempty"`
}

type ReactionType struct {
	Type          string `json:"type"`
	Emoji         string `json:"emoji,omitempty"`
	CustomEmojiID string `json:"custom_emoji_id,omitempty"`
}

type MessageReactionUpdated struct {
	Chat        Chat           `json:"chat"`
	MessageID   int64          `json:"message_id"`
	User        *User          `json:"user,omitempty"`
	Date        int64          `json:"date"`
	OldReaction []ReactionType `json:"old_reaction"`
	NewReaction []ReactionType `json:"new_reaction"`
}

type Update struct {
	UpdateID        int64                   `json:"update_id"`
	Message         *Message                `json:"message,omitempty"`
	EditedMessage   *Message                `json:"edited_message,omitempty"`
	MessageReaction *MessageReactionUpdated `json:"message_reaction,omitempty"`
}

type File struct {
	FileID   string `json:"file_id"`
	FileSize int64  `json:"file_size,omitempty"`
	FilePath string `json:"file_path,omitempty"`
}
