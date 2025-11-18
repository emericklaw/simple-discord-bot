package main

import (
	"fmt"

	"github.com/bwmarrin/discordgo"
	"github.com/go-co-op/gocron/v2"
	"github.com/google/uuid"
	"github.com/spf13/viper"
)

var scheduledMessagesScheduler gocron.Scheduler
var scheduledMessagesJobs map[string]gocron.Job = make(map[string]gocron.Job)

func initScheduledMessages() {
	var err error
	if scheduledMessagesScheduler != nil {
		logger("info", "Shutting down existing Scheduled Message scheduler")
		scheduledMessagesScheduler.Shutdown()
		scheduledMessagesScheduler = nil
	}
	if scheduledMessagesScheduler == nil {
		logger("info", "Creating Scheduled Message scheduler")

		scheduledMessagesScheduler, err = gocron.NewScheduler()
		if err != nil {
			logger("error", "Failed to create new Scheduled Message scheduler: %s", err)
			return
		}
	}

	// Schedule all messages from config
	scheduledMessages := viper.GetStringMap("scheduled_messages")
	for scheduledMessageID, _ := range scheduledMessages {
		scheduleScheduledMessages(scheduledMessageID)
	}

	// start the scheduler
	scheduledMessagesScheduler.Start()
	logger("info", "Scheduled Message scheduler started")
}

func scheduleScheduledMessages(scheduledMessageID string) {

	logger("debug", "Scheduling scheduled message %s", scheduledMessageID)
	if !viper.IsSet("scheduled_messages." + scheduledMessageID) {
		logger("error", "Scheduled message ID not found: %s", scheduledMessageID)
		return
	}

	if viper.IsSet("scheduled_messages." + scheduledMessageID + ".archived") {
		if viper.GetBool("scheduled_messages." + scheduledMessageID + ".archived") {
			logger("debug", "Scheduled message ID %s is archived, not scheduling", scheduledMessageID)
			return
		}
	}

	schedule := viper.GetString("scheduled_messages." + scheduledMessageID + ".schedule")
	name := viper.GetString("scheduled_messages." + scheduledMessageID + ".name")

	job, err := scheduledMessagesScheduler.NewJob(
		gocron.CronJob(
			schedule,
			false,
		),
		gocron.NewTask(
			func(a string) {
				sendScheduledMessage(dg, a)
			},
			scheduledMessageID,
		),
		gocron.WithName(name),
		gocron.WithIdentifier(uuid.MustParse(scheduledMessageID)),
	)
	if err != nil {
		logger("error", "Error scheduling scheduled message - Scheduled Message ID: %s - Error: %s", scheduledMessageID, err)
		return
	}

	jobs[scheduledMessageID] = job

	logger("info", "Scheduled message '%s' scheduled - ID: %s Cron: %s", name, scheduledMessageID, schedule)
}

// Adds a new scheduled message to config and schedules it
func scheduledMessagesAdd(s *discordgo.Session, m *discordgo.MessageCreate, command string, content string) {
	parts := splitQuotedParts(content)

	if len(parts) < 4 {
		privateMessageCreate(s, m.Author.ID, "Invalid command format. Usage: <name> <cron> <channel_id> <message>", false)
		return
	}

	newScheduledMessageID := uuid.New().String()
	newScheduledMessage := "scheduled_messages." + newScheduledMessageID

	name := parts[0]
	cron := parts[1]
	channelID := parts[2]
	message := parts[3]

	viper.Set(newScheduledMessage+".name", name)
	viper.Set(newScheduledMessage+".schedule", cron)
	viper.Set(newScheduledMessage+".channel_id", channelID)
	viper.Set(newScheduledMessage+".message", message)
	viper.WriteConfig()

	privateMessageCreate(s, m.Author.ID, "Scheduled message added: "+name, false)
	logger("info", "Scheduled message added: %s", name)
}

// Lists scheduled scheduled messages
func scheduledMessagesListScheduled(s *discordgo.Session, m *discordgo.MessageCreate, command string, content string) {
	if scheduledMessagesScheduler == nil {
		logger("error", "Scheduled Message scheduler is not initialised")
		privateMessageCreate(s, m.Author.ID, "Scheduled Message scheduler is not initialised", false)
		return
	}
	scheduledMessagesJobs := scheduledMessagesScheduler.Jobs()
	if len(scheduledMessagesJobs) == 0 {
		logger("debug", "No scheduled messages found")
		privateMessageCreate(s, m.Author.ID, "No scheduled messages found", false)
		return
	}
	logger("info", "Scheduled messages found")
	var jobItems string
	for _, job := range scheduledMessagesJobs {
		jobID := job.ID()
		jobName := job.Name()
		jobNextRun, _ := job.NextRun()
		jobLastRun, lastRunExists := job.LastRun()

		if lastRunExists != nil || jobLastRun.IsZero() {
			jobItems += fmt.Sprintf(
				"## Name: %s\n**ID:** %s\n**Next Run:** %s\n\n",
				jobName,
				jobID,
				jobNextRun.In(timezone).Format("2006-01-02")+" "+jobNextRun.In(timezone).Format("15:04:05"),
			)
		} else {
			jobItems += fmt.Sprintf(
				"## Name: %s\n**ID:** %s\n**Next Run:** %s\n**Last Run:** %s\n\n",
				jobName,
				jobID,
				jobNextRun.In(timezone).Format("2006-01-02")+" "+jobNextRun.In(timezone).Format("15:04:05"),
				jobLastRun.In(timezone).Format("2006-01-02")+" "+jobLastRun.In(timezone).Format("15:04:05"),
			)
		}
	}
	if jobItems == "" {
		privateMessageCreate(s, m.Author.ID, "No scheduled messages found", false)
		return
	}
	privateMessageCreate(s, m.Author.ID, "# Scheduled Messages\n"+jobItems, false)
	logger("debug", "Scheduled scheduled messages sent")
}

// Lists all scheduled messages
func scheduledMessagesList(s *discordgo.Session, m *discordgo.MessageCreate, command string, content string) {
	messages := viper.GetStringMap("scheduled_messages")
	if len(messages) == 0 {
		privateMessageCreate(s, m.Author.ID, "No scheduled messages found", false)
		return
	}
	var items string
	for id, msg := range messages {
		msgMap, ok := msg.(map[string]interface{})
		if !ok {
			continue
		}
		archived := msgMap["archived"]
		if archived != nil && archived.(bool) {
			continue
		}

		schedule := msgMap["schedule"]
		channelID := msgMap["channel_id"]
		message := msgMap["message"]
		name := msgMap["name"]
		items += fmt.Sprintf("## Name: %s\n**ID:** %s\n**Schedule:** `%v`\n**Channel:** %v\n**Message:** %v\n\n", name, id, schedule, channelID, message)
	}
	if items == "" {
		privateMessageCreate(s, m.Author.ID, "No scheduled messages found", false)
		return
	}
	privateMessageCreate(s, m.Author.ID, "# Scheduled Messages\n"+items, false)
	logger("debug", "Scheduled messages list sent")
}

// Archives a scheduled message
func scheduledMessagesArchive(s *discordgo.Session, m *discordgo.MessageCreate, command string, content string) {
	name := content
	key := "scheduled_messages." + name
	if !viper.IsSet(key) {
		privateMessageCreate(s, m.Author.ID, "Scheduled message not found: "+name, false)
		return
	}
	viper.Set(key+".archived", true)
	viper.WriteConfig()
	privateMessageCreate(s, m.Author.ID, "Scheduled message archived: "+name, false)
	logger("warning", "Scheduled message archived: %s", name)
}

// Runs a scheduled message immediately
func scheduledMessagesRun(s *discordgo.Session, m *discordgo.MessageCreate, command string, content string) {
	if content == "" {
		privateMessageCreate(s, m.Author.ID, "No scheduled message ID provided", false)
		return
	}

	scheduledMessageID := content

	if !viper.IsSet("scheduled_messages." + scheduledMessageID) {
		privateMessageCreate(s, m.Author.ID, "Scheduled message ID not found", false)
		return
	}

	if scheduledMessagesScheduler == nil {
		logger("error", "Scheduled messages scheduler is not initialised")
		privateMessageCreate(s, m.Author.ID, "Scheduled messages scheduler is not initialised", false)
		return
	}

	sendScheduledMessage(s, scheduledMessageID)
}

// Sends a scheduled message to the configured channel
func sendScheduledMessage(s *discordgo.Session, scheduledMessageID string) {
	scheduledMessage := viper.GetStringMap("scheduled_messages." + scheduledMessageID)
	channelID, _ := scheduledMessage["channel_id"].(string)
	name, _ := scheduledMessage["name"].(string)
	messageText, _ := scheduledMessage["message"].(string)

	_, err := s.ChannelMessageSend(channelID, messageText)
	if err != nil {
		logger("error", "Failed to send scheduled message '%s' to channel %s: %s", name, channelID, err)
	} else {
		logger("info", "Scheduled message '%s' sent to channel %s", name, channelID)
	}
}
