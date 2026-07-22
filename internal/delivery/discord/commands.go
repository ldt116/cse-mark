package discord

import "github.com/bwmarrin/discordgo"

// commandName is the set of slash commands the Discord bot registers.
const (
	cmdCreate = "create"
	cmdSync   = "sync"
	cmdBind   = "bind"
	cmdMark   = "mark"
	cmdProfile = "profile"
)

// optCourse, optCsvURL, optCourseID are option names.
const (
	optCourse  = "course-id"
	optCsvURL  = "csv-url"
	optCourse2 = "course"
)

// applicationCommands returns the slash-command definitions registered for the
// guild. Guild commands apply immediately (vs. global commands' ~1h delay),
// matching the dev/test-first choice in the design.
func applicationCommands() []*discordgo.ApplicationCommand {
	return []*discordgo.ApplicationCommand{
		{
			Name:        cmdCreate,
			Description: "Tạo/cập nhật lớp + import điểm + tạo role/channel",
			Options: []*discordgo.ApplicationCommandOption{
				{Name: optCourse, Description: "Mã lớp (vd CO2003-L01)", Type: discordgo.ApplicationCommandOptionString, Required: true},
				{Name: optCsvURL, Description: "URL bảng điểm CSV", Type: discordgo.ApplicationCommandOptionString, Required: true},
			},
		},
		{
			Name:        cmdSync,
			Description: "Tải lại CSV của lớp và đồng bộ lại",
			Options: []*discordgo.ApplicationCommandOption{
				{Name: optCourse, Description: "Mã lớp", Type: discordgo.ApplicationCommandOptionString, Required: true},
			},
		},
		// bind/mark/profile are registered here but wired in M5; their handlers
		// are no-ops until then so the command surface is coherent.
		{Name: cmdBind, Description: "Liên kết tài khoản với MSSV (email OTP)"},
		{Name: cmdMark, Description: "Tra cứu điểm (ephemeral)", Options: []*discordgo.ApplicationCommandOption{
			{Name: optCourse, Description: "Mã lớp (bỏ trống để xem tất cả)", Type: discordgo.ApplicationCommandOptionString, Required: false},
		}},
		{Name: cmdProfile, Description: "Hồ sơ của bạn (ephemeral)"},
	}
}

// commandCustomID prefixes for modal/button interactions (bind flow, M5).
const (
	cidBindEmailModal = "bind:email:modal" // modal custom id for email input
	cidBindVerifyBtn  = "bind:verify:btn"  // button: open OTP modal
	cidBindOtpModal   = "bind:otp:modal:"  // modal prefix for OTP input (carries email)
)
