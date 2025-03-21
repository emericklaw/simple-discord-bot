package main

import (
	"sort"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/spf13/viper"
)

const inductionRequestThreadTitle string = "How to request an induction for a workshop or tool"
const inductionRequestThreadMessage string = "Please use the buttons in this message to request an induction for a workshop or a tool. Someone who can induct you will be in touch soon."

func checkInductions(s *discordgo.Session) {
	logger("info", "Checking induction request thread")
	if viper.IsSet("discord_inductions.request_message_id") {
		requestMessageID := viper.GetString("discord_inductions.request_message_id")

		_, err := dg.ChannelMessage(requestMessageID, requestMessageID)

		if err != nil {
			if err.(*discordgo.RESTError).Message.Code == 10008 || err.(*discordgo.RESTError).Message.Code == 0 || err.(*discordgo.RESTError).Message.Code == 10003 {
				logger("warning", "Induction request message not found")
				updateInductionMessage(s, "")
				return
			} else {
				logger("error", "Could not find induction request message: %s", err)
			}
		}

		updateInductionMessage(s, requestMessageID)

	} else {
		logger("warning", "No induction request message found")
		updateInductionMessage(s, "")
	}
}

func checkInductionsCommand(s *discordgo.Session, m *discordgo.MessageCreate, command string, content string) {
	checkInductions(s)
	logger("warning", "Induction request thread checked")
	privateMessageCreate(s, m.Author.ID, "Induction request thread updated", false)
}

func updateInductionMessage(s *discordgo.Session, requestMessageID string) {
	logger("info", "Updating induction request thread")

	inductionRequestChannelID := viper.GetString("discord_inductions.request_channel_id")

	guildID := viper.GetString("_discord_default_server_id")

	logger("debug", "Getting members")
	members, errMembers := dg.GuildMembers(guildID, "", 1000)
	if errMembers != nil {
		logger("error", "Could not fetch guild members %s", errMembers)
	} else {
		sort.Slice(members, func(i, j int) bool {
			return getDiscordDisplayName(members[i]) < getDiscordDisplayName(members[j])
		})
	}

	logger("debug", "Getting roles")
	roles, err := s.GuildRoles(guildID)
	if err != nil {
		logger("error", "Error getting guild roles: %s", err)
	} else {
		// Sort roles by role.Name
		sort.Slice(roles, func(i, j int) bool {
			return roles[i].Name < roles[j].Name
		})
	}

	embeds := []*discordgo.MessageEmbed{}
	fields := []*discordgo.MessageEmbedField{}
	components := []discordgo.MessageComponent{}

	lastCheckedRoleGroup := ""
	actionRow := discordgo.ActionsRow{}

	logger("debug", "Checking roles")
	for _, role := range roles {
		if strings.HasPrefix(role.Name, "Induction -") {

			if lastCheckedRoleGroup != strings.TrimSpace(strings.Split(role.Name, "-")[1]) {
				if lastCheckedRoleGroup != "" {
					components = append(components, actionRow)
					actionRow = discordgo.ActionsRow{Components: []discordgo.MessageComponent{}}
				}

				lastCheckedRoleGroup = strings.TrimSpace(strings.Split(role.Name, "-")[1])
			}

			if strings.Count(role.Name, "-") == 1 {

				if errMembers == nil {
					membersWithRole := ""
					for _, member := range members {
						for _, memberRole := range member.Roles {
							if memberRole == role.ID {
								membersWithRole = membersWithRole + "<@" + member.User.ID + ">\n"
								break
							}
						}
					}

					newField := &discordgo.MessageEmbedField{
						Name:   strings.TrimSpace(strings.Split(role.Name, "-")[1]),
						Value:  membersWithRole,
						Inline: true,
					}
					fields = append(fields, newField)
				}

				actionButton := discordgo.Button{
					Label:    strings.TrimSpace(strings.Replace(strings.Split(role.Name, "-")[1], " DISABLED", "", 1)),
					Style:    discordgo.DangerButton,
					CustomID: role.ID,
					Disabled: strings.HasSuffix(role.Name, " DISABLED"),
				}
				actionRow.Components = append(actionRow.Components, actionButton)

			} else {
				actionButton := discordgo.Button{
					Label:    strings.TrimSpace(strings.Replace(strings.Split(role.Name, "-")[2], " DISABLED", "", 1)),
					Style:    discordgo.PrimaryButton,
					CustomID: role.ID,
					Disabled: strings.HasSuffix(role.Name, " DISABLED"),
				}
				actionRow.Components = append(actionRow.Components, actionButton)
			}
		}
	}

	// add last action row
	components = append(components, actionRow)

	if errMembers == nil {
		embeds = append(embeds, &discordgo.MessageEmbed{
			Title:  "Inductors",
			Fields: fields,
			Color:  0xCF142B,
		})
	}

	if requestMessageID != "" {
		logger("info", "Editing induction request thread")

		message := discordgo.MessageEdit{
			ID:         requestMessageID,
			Channel:    requestMessageID,
			Embeds:     &embeds,
			Components: &components,
		}

		_, err = dg.ChannelEdit(requestMessageID, &discordgo.ChannelEdit{
			Name: inductionRequestThreadTitle,
		})
		if err != nil {
			logger("error", "Could not edit induction request thread title: %v", err)
		}

		_, err := s.ChannelMessageEditComplex(&message)
		if err != nil {
			logger("error", "Could not edit induction request message: %s", err)
		}

		logger("info", "Edited induction request thread")

	} else {

		logger("info", "Creating induction request thread")

		threadData := discordgo.ThreadStart{
			Name:                inductionRequestThreadTitle,
			AutoArchiveDuration: 60 * 24 * 7,
			Type:                13,
		}
		message := discordgo.MessageSend{
			Content:    inductionRequestThreadMessage,
			Embeds:     embeds,
			Components: components,
		}

		msg, err := s.ForumThreadStartComplex(inductionRequestChannelID, &threadData, &message)
		if err != nil {
			logger("error", "Error creating induction request message: %s", err)
		}

		requestMessageID = msg.ID

		viper.Set("discord_inductions.request_message_id", requestMessageID)
		viper.WriteConfig()

		logger("info", "Created induction request thread")

	}

	logger("info", "Pinning induction request thread")
	threadFlags := discordgo.ChannelFlagPinned
	_, err = dg.ChannelEdit(requestMessageID, &discordgo.ChannelEdit{
		Flags: &threadFlags,
	})
	if err != nil {
		logger("error", "Could not pin induction request thread: %v", err)
	}

}

func interactionHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	switch i.Type {
	case discordgo.InteractionMessageComponent:

		guildID := viper.GetString("_discord_default_server_id")
		inductionRequestChannelID := viper.GetString("discord_inductions.request_channel_id")
		role, _ := dg.State.Role(guildID, i.MessageComponentData().CustomID)

		titleText := getDiscordDisplayName(i.Member) + " would like an induction for " + strings.SplitN(role.Name, " - ", 2)[1]
		messageText := "<@" + i.Member.User.ID + "> would like an induction for " + strings.SplitN(role.Name, " - ", 2)[1] + ". Please can someone help them out? <@&" + i.MessageComponentData().CustomID + ">"

		outstandingTagID := viper.GetString("discord_inductions.outstanding_tag_id")

		threadData := discordgo.ThreadStart{
			Name:                titleText,
			AutoArchiveDuration: 60 * 24 * 7,
			Type:                13,
			AppliedTags:         []string{outstandingTagID},
		}
		message := discordgo.MessageSend{
			Content: messageText,
		}

		thread, errT := s.ForumThreadStartComplex(inductionRequestChannelID, &threadData, &message)
		if errT != nil {
			logger("error", "Error creating induction request thread: %s", errT)
		}

		logger("debug", "Induction user request thread created: %s", thread.ID)
		err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "A induction request has been made for " + strings.SplitN(role.Name, " - ", 2)[1] + ". Please keep an eye out for a reply from someone that can induct you here <#" + thread.ID + ">",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		if err != nil {
			logger("error", "Error sending induction request response message to user: %s", err)
		}
	}
}

// Event handler for ThreadUpdate
func threadUpdate(s *discordgo.Session, event *discordgo.ThreadUpdate) {

	logger("debug", "ThreadUpdate event received")
	if event.Channel.Type == discordgo.ChannelTypeGuildPublicThread || event.Channel.Type == discordgo.ChannelTypeGuildPrivateThread {

		// Check if the parent channel is the induction request channel
		logger("debug", "ThreadUpdate event is a thread")
		if event.Channel.ParentID == viper.GetString("discord_inductions.request_channel_id") {
			logger("debug", "ThreadUpdate event is in the induction request channel")

			// Check if the updated thread is not the induction request message thread
			if event.Channel.ID != viper.GetString("discord_inductions.request_message_id") {
				added, removed := diffArrays(event.BeforeUpdate.AppliedTags, event.Channel.AppliedTags)
				logger("debug", "ThreadUpdate Added Tags: %s", added)
				logger("debug", "ThreadUpdate Removed Tags: %s", removed)
				if sliceContainsValue(added, viper.GetString("discord_inductions.booked_tag_id")) {
					logger("info", "Induction thread marked as booked")
				}
				if sliceContainsValue(added, viper.GetString("discord_inductions.completed_tag_id")) {
					logger("info", "Induction thread marked as completed")
				}

				if sliceContainsValue(added, viper.GetString("discord_inductions.booked_tag_id")) || sliceContainsValue(added, viper.GetString("discord_inductions.completed_tag_id")) {
					err := removeTagFromThread(dg, event.Channel.ID, viper.GetString("discord_inductions.outstanding_tag_id"))
					if err != nil {
						logger("error", "Could not remove tag from thread ThreadID: %s\nError: %s", event.Channel.ID, err)
					} else {
						logger("info", "Tag removed successfully")
					}

				}
				if sliceContainsValue(added, viper.GetString("discord_inductions.completed_tag_id")) {
					threadArchived := true
					_, err := s.ChannelEdit(event.Channel.ID, &discordgo.ChannelEdit{
						Archived: &threadArchived,
					})
					if err != nil {
						logger("error", "Could not close thread ThreadID: %s\nError: %s", event.Channel.ID, err)
					} else {
						logger("info", "Thread closed successfully")
					}
				}
			}
		}
	}
}
