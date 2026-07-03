package discordmapping

// Model stores the Discord role/channel ids provisioned for a course. CourseId
// is the primary key (_id) and matches Class.CourseId.
type Model struct {
	CourseId         string `json:"course_id"          bson:"_id"`
	DiscordRoleId    string `json:"discord_role_id"    bson:"discord_role_id"`
	DiscordChannelId string `json:"discord_channel_id" bson:"discord_channel_id"`
}
