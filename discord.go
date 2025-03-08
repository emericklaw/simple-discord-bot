package main

import (
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/spf13/viper"
)

func initDiscord() error {
	log.Println("🔹 Starting Discord initialization...")

	BotToken := viper.GetString("_discord_token")

	var err error
	dg, err = discordgo.New("Bot " + BotToken)
	if err != nil {
		logger("emergency", "Unable to create Discord session: %s", err)
		return err
	}

	logger("debug", "Discord session object created")

	err = dg.Open()
	if err != nil {
		logger("emergency", "Unable to open Discord connection: %s", err)
		return err
	}

	logger("info", "Bot is connected to Discord!")

	dg.AddHandler(messageCreate)
	dg.AddHandler(addReaction)
	dg.AddHandler(removeReaction)

	return nil
}

// discord message handler
func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	// ignore messages from itself
	if m.Author.ID == s.State.User.ID {
		return
	}

	chanl, err := s.Channel(m.ChannelID)
	if err != nil {
		return
	}

	guild, _ := s.Guild(chanl.GuildID)
	var author *discordgo.Member

	if guild != nil {
		author, _ = s.GuildMember(guild.ID, m.Author.ID)
	} else {
		author, _ = s.GuildMember(viper.GetString("_discord_default_server_id"), m.Author.ID)
	}

	// ignore commands we don't care about
	if !strings.HasPrefix(strings.ToLower(m.Content), strings.ToLower(viper.GetString("_command_key"))+" ") {
		return
	}

	// log commands passed to bot
	logger("info", "User: %s ID: %s Command: \"%s\"", m.Author.Username, m.Author.ID, m.Content)

	// strip out the command key
	cleancommand := strings.Replace(strings.ToLower(m.Content), viper.GetString("_command_key")+" ", "", 1)

	// mycommand = the valid command found
	// iscommandvalid = is command valid?
	// commandoptions = map of all options, ready for templating
	mycommand, iscommandvalid, commandoptions := findCommand(cleancommand)

	if !iscommandvalid {
		logger("warning", "User: %s ID: %s Command: \"%s\" Status: \"Command is invalid\"", m.Author.Username, m.Author.ID, m.Content)
		return
	}

	// find role for the primary command
	commandRoles := viper.GetStringSlice("commands." + mycommand + ".roles")

	// check if a role has been assigned to the command, and ignore if none has been set or role is invalid
	for _, role := range commandRoles {
		if !isRoleValid(role) {
			// role doesn't exist
			logger("error", "Role (%s) not valid do not exist for command %s", role, mycommand)
			return
		}
	}

	// check if user has permission to execute a command
	var canRun bool = false
	for _, role := range commandRoles {
		if checkUserPerms(role, author, m.Author.ID) {
			canRun = true
		}
	}
	if !canRun {
		logger("error", "User: %s ID: %s Does not have permission to run Command: \"%s\"", m.Author.Username, m.Author.ID, m.Content)
		return
	}

	// check if command is valid and do appropriate text response
	if _, ok := viper.GetStringMap("commands")[mycommand]; ok {

		ismessage := viper.IsSet("commands." + mycommand + ".message")
		isapicall := viper.IsSet("commands." + mycommand + ".api")
		isfile := viper.IsSet("commands." + mycommand + ".file")
		isshell := viper.IsSet("commands." + mycommand + ".shell")
		isfunction := viper.IsSet("commands." + mycommand + ".function")
		issecret := viper.GetBool("commands." + mycommand + ".secret")

		// if api and file then return and throw an error, this is not a valid option configuration
		if isapicall && isfile {
			logger("error", "Cannot have command api with file on command %s", mycommand)
			return
		}

		// if shell and (file or api) then return and throw an error, this is not a valid option configuration
		if isshell && (isfile || isapicall) {
			logger("error", "Cannot have command shell with file or api on command %s", mycommand)
			return
		}

		// if function and (file or api or shell) then return and throw an error, this is not a valid option configuration
		if isfunction && (isshell || isfile || isapicall) {
			logger("error", "Cannot have command function with shell or file or api on command %s", mycommand)
			return
		}

		var messagetosend string

		if ismessage {
			messagetosend = prepareTemplate(viper.GetString("commands."+mycommand+".message"), commandoptions)
		} else if isapicall {
			// if an api call do it and get response which will become the message sent to the user
			messagetosend = downloadApi(prepareTemplate(viper.GetString("commands."+mycommand+".api"), commandoptions))

		} else if isfile {
			// if we need to load a files contents into message to send
			tempcontents, err := loadFile(prepareTemplate(viper.GetString("commands."+mycommand+".file"), commandoptions))
			if err != nil {
				logger("warning", "Error loading file: %s with: %v", messagetosend, err)
				return
			}

			messagetosend = tempcontents
		} else if isshell && viper.GetBool("_shell_enabled") {
			err, stdout, stderr := shellOut(prepareTemplate(viper.GetString("commands."+mycommand+".shell"), commandoptions))
			if err != nil {
				logger("error", "Error executing command: \"%s\" err: %v", messagetosend, err)
			}

			messagetosend = ""
			if len(stdout) > 0 {
				messagetosend = messagetosend + stdout
			}
			if len(stderr) > 0 {
				messagetosend = messagetosend + "\nSTDERR:\n-------\n" + stderr
			}
			if len(stderr) > 0 {
				messagetosend = messagetosend + "\nSTDERR:\n-------\n" + stderr
			}
			//messagetosend = messagetosend + "```\n"

			// if messagetosend is empty, do nothing and return
			if len(messagetosend) == 8 {
				return
			}
		} else if isshell && !viper.GetBool("_shell_enabled") {
			// do nothing and return when command is a shell and _shell_enabled = false
			logger("error", "Cannot run shell command when _shell_enabled = false")
			return
		} else if isfunction {
			lengthOfMessageWithoutCommand := len(viper.GetString("_command_key")) + 1 + len(mycommand) + 1
			var message string
			if lengthOfMessageWithoutCommand > len(m.Content) {
				message = ""
			} else {
				message = m.Content[lengthOfMessageWithoutCommand:]
			}

			functionName := prepareTemplate(viper.GetString("commands."+mycommand+".function"), commandoptions)
			// Map function names to actual functions
			functions := map[string]func(*discordgo.Session, *discordgo.MessageCreate, string, string){
				"sendMessage":      sendMessage,
				"editMessage":      editMessage,
				"listEmoji":        listEmoji,
				"showHelp":         showHelp,
				"apiHomeAssistant": apiHomeAssistant,
				"cameraSnapshot":   cameraSnapshot,
				"cameraList":       cameraList,
				"showVersion":      showVersion,
				"logLevelSet":      logLevelSet,
				"logLevelShow":     logLevelShow,
			}

			// Call the function based on the name
			if function, ok := functions[functionName]; ok {
				function(s, m, mycommand, message)
			} else {
				logger("warning", "Function "+functionName+" not found")
			}

		}

		var usewrapper = false

		if isshell || isfile {
			usewrapper = true
		}

		// send the command response, if marked as secret send via private message do not send if command is a custom function
		if !isfunction {
			if issecret {
				privateMessageCreate(s, m.Author.ID, messagetosend, usewrapper)
			} else {
				channelMessageCreate(s, m, messagetosend, usewrapper)
			}
		}

		return
	}
}

// discord addReaction handler
func addReaction(s *discordgo.Session, mr *discordgo.MessageReactionAdd) {
	for _, v := range viper.GetStringMap("discord_reactions") {
		if m, ok := v.(map[string]interface{}); ok {
			// check message id is being tracked
			if strconv.Itoa(m["message_id"].(int)) == mr.MessageID {

				// check emoji is being tracked for this message
				emoji := strings.Split(m["emoji"].(string), ":")
				if emoji[0] == mr.Emoji.Name {
					// check which type of reaction this is
					if m["type"] == "role" {
						// add role
						s.GuildMemberRoleAdd(mr.GuildID, mr.UserID, strconv.Itoa(m["role_id"].(int)))
					}
				}
			}
		} else {
			logger("warning", "Data is not a map[string]interface{}")
		}
	}
}

// discord removeReaction handler
func removeReaction(s *discordgo.Session, mr *discordgo.MessageReactionRemove) {
	for _, v := range viper.GetStringMap("discord_reactions") {
		if m, ok := v.(map[string]interface{}); ok {
			// check message id is being tracked
			if strconv.Itoa(m["message_id"].(int)) == mr.MessageID {

				// check emoji is being tracked for this message
				emoji := strings.Split(m["emoji"].(string), ":")
				if emoji[0] == mr.Emoji.Name {
					// check which type of reaction this is
					if m["type"] == "role" {
						// remove role
						s.GuildMemberRoleRemove(mr.GuildID, mr.UserID, strconv.Itoa(m["role_id"].(int)))
					}
				}
			}
		} else {
			logger("warning", "Data is not a map[string]interface{}")
		}
	}
}

// check reactions
func checkReactions(s *discordgo.Session) {
	logger("info", "Checking reactions for tracked messages")
	for _, v := range viper.GetStringMap("discord_reactions") {
		if m, ok := v.(map[string]interface{}); ok {
			channelID := strconv.Itoa(m["channel_id"].(int))
			messageID := strconv.Itoa(m["message_id"].(int))

			// check emoji is being tracked for this message
			messageReactions, err := s.MessageReactions(channelID, messageID, m["emoji"].(string), 100, "", "")
			if err != nil {
				logger("error", "Checking reactions channelID: %s messageID: %s, Error: %s", channelID, messageID, err)
			}
			var hasBotReaction bool = false
			for _, user := range messageReactions {
				if user.ID == s.State.User.ID {
					hasBotReaction = true
				}
			}

			if !hasBotReaction {
				s.MessageReactionAdd(channelID, messageID, m["emoji"].(string))
				// pause to make sure reactions are added in order
				time.Sleep(1 * time.Second)
			}

		}
	}
}

// send a private message to a user
func privateMessageCreate(s *discordgo.Session, userid string, message string, codeblock bool) {
	var wrapper string
	if codeblock {
		wrapper = "```"
	}

	// create the private message channel to user
	channel, err := s.UserChannelCreate(userid)
	if err != nil {
		logger("error", "Creating PM channel to %s with %s", userid, err)
		s.ChannelMessageSend(userid, "Something went wrong while sending the DM!")
		return
	}

	if len(message) > viper.GetInt("_discord_message_chunk_size") {
		messagechunks := chunkMessage(message, viper.GetString("_discord_message_split_char"), viper.GetInt("_discord_message_chunk_size"))

		var allkeys []int

		for k, _ := range messagechunks {
			allkeys = append(allkeys, k)
		}

		sort.Ints(allkeys[:])

		for _, key := range allkeys {
			_, err = s.ChannelMessageSend(channel.ID, wrapper+messagechunks[key]+wrapper)
			// todo: catch errors here
		}

	} else {
		// send the message to the user
		_, err = s.ChannelMessageSend(channel.ID, wrapper+message+wrapper)
		if err != nil {
			logger("error", "Cannot send DM to %s with %s", userid, err)
			s.ChannelMessageSend(userid, "Failed to send you a DM. Did you disable DM in your privacy settings?")
		}
	}

}

// send a message to a channel
func channelMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate, message string, codeblock bool) {
	var wrapper string
	if codeblock {
		wrapper = "```"
	}

	var err error

	if len(message) > viper.GetInt("_discord_message_chunk_size") {
		messagechunks := chunkMessage(message, viper.GetString("_discord_message_split_char"), viper.GetInt("_discord_message_chunk_size"))
		var allkeys []int
		for k, _ := range messagechunks {
			allkeys = append(allkeys, k)
		}
		sort.Ints(allkeys[:])

		for _, key := range allkeys {
			_, err = s.ChannelMessageSend(m.ChannelID, wrapper+messagechunks[key]+wrapper)
			// todo: handle error
		}

	} else {

		// send the message to the user
		_, err = s.ChannelMessageSend(m.ChannelID, wrapper+message+wrapper)
		if err != nil {
			logger("error", "Cannot send message to channel: %s", err)
		}
	}

}
