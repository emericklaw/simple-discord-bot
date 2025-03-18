package main

import (
	"sort"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/spf13/viper"
)

// check inductions
func checkInductions(s *discordgo.Session) {
	logger("info", "Checking inductions")
	if viper.IsSet("discord_inductions.request_message_id") {
		inductionRequestChannelID := viper.GetString("discord_inductions.request_channel_id")
		requestMessageID := viper.GetString("discord_inductions.request_message_id")
		_, err := dg.ChannelMessage(inductionRequestChannelID, requestMessageID)

		if err != nil {
			if err.(*discordgo.RESTError).Message.Code == 10008 {
				logger("warning", "Induction request message not found")
				createInductionMessage(s, "")
				return
			} else {
				logger("error", "Error finding induction request message: %s", err)
			}
		}

		createInductionMessage(s, requestMessageID)
		if err != nil {
			logger("error", "Error editing induction request message: %s", err)
		}

	} else {
		logger("warning", "No induction request message found")

		createInductionMessage(s, "")

	}
}

func createInductionMessage(s *discordgo.Session, requestMessageID string) {
	logger("warning", "Creating induction message")
	inductionRequestChannelID := viper.GetString("discord_inductions.request_channel_id")

	guildID := viper.GetString("_discord_default_server_id")

	members, err := dg.GuildMembers(guildID, "", 1000)
	if err != nil {
		logger("error", "Could not fetch guild members %s", err)
		return
	}
	sort.Slice(members, func(i, j int) bool {
		return members[i].User.GlobalName < members[j].User.GlobalName
	})

	embeds := []*discordgo.MessageEmbed{}
	fields := []*discordgo.MessageEmbedField{}
	components := []discordgo.MessageComponent{}

	roles, err := s.GuildRoles(guildID)
	if err != nil {
		logger("error", "Error getting guild roles: %s", err)
	}
	// Sort roles by role.Name
	sort.Slice(roles, func(i, j int) bool {
		return roles[i].Name < roles[j].Name
	})

	lastCheckedRoleGroup := ""
	// lastCheckedRoleName := ""
	actionRow := discordgo.ActionsRow{}

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

	embeds = append(embeds, &discordgo.MessageEmbed{
		Title:       "Induction Requests",
		Description: "Please use the buttons below to request an induction on a workshop or a tool and someone who can induct you will be in touch soon.",
		Fields:      fields,
		Color:       0xCF142B,
	})

	if requestMessageID != "" {
		// Edit existing message
		message := discordgo.MessageEdit{
			ID:         requestMessageID,
			Channel:    inductionRequestChannelID,
			Embeds:     &embeds,
			Components: &components,
		}

		_, err := s.ChannelMessageEditComplex(&message)
		if err != nil {
			logger("error", "Error creating induction request message: %s", err)
		}

	} else {
		// Send the message to the specified channel

		message := discordgo.MessageSend{
			Embeds:     embeds,
			Components: components,
		}

		msg, err := s.ChannelMessageSendComplex(inductionRequestChannelID, &message)
		if err != nil {
			logger("error", "Error creating induction request message: %s", err)
		}
		msgID := msg.ID

		viper.Set("discord_inductions.request_message_id", msgID)
		viper.WriteConfig()
	}
}

func interactionHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	switch i.Type {
	case discordgo.InteractionMessageComponent:

		guildID := viper.GetString("_discord_default_server_id")
		inductionDiscussionChannelID := viper.GetString("discord_inductions.discussion_channel_id")
		role, _ := dg.State.Role(guildID, i.MessageComponentData().CustomID)

		sendMessageToDiscord(inductionDiscussionChannelID, "<@"+i.Member.User.ID+"> has asked for an induction for "+strings.SplitN(role.Name, " - ", 2)[1]+". Please can someone help them out? <@&"+i.MessageComponentData().CustomID+">")
		err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "A request has been made for an induction on the " + strings.SplitN(role.Name, " - ", 2)[1] + ". Please keep an eye out for a reply from someone that can induct you in <#" + inductionDiscussionChannelID + ">",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		if err != nil {
			panic(err)
		}
	}
}
