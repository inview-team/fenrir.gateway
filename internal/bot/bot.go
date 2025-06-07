package bot

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"chatops-bot/internal/models"
	"chatops-bot/internal/service"

	"gopkg.in/telebot.v3"
)

// Callback Prefixes
const (
	viewIncidentPrefix          = "vi:"
	showActionsPrefix           = "sa:"
	performActionPrefix         = "pa:"
	closeIncidentPrefix         = "ci:"
	setStatusPrefix             = "ss:"
	viewResourcePrefix          = "vr:"
	performResourceActionPrefix = "pra:"
	scaleDeploymentPrefix       = "scd:"
	allocateHardwarePrefix      = "ahw:"
	toggleHistoryPrefix         = "th:"
)

// State for awaiting user input
type awaitingInputState struct {
	Request   *models.ActionRequest
	MessageID int
	ChatID    int64
}

type userState struct {
	AwaitingRejectReasonFor    uint
	AwaitingReplicaCountFor    *awaitingInputState
	AwaitingHardwareRequestFor *awaitingInputState
}

type Bot struct {
	bot            *telebot.Bot
	service        *service.IncidentService
	userRepo       service.UserRepository
	suggester      *service.ActionSuggester
	userStates     map[int64]*userState
	mu             sync.RWMutex
	viewRegistry   map[uint]map[string]telebot.Editable
	registryMu     sync.RWMutex
	alertChannelID int64
}

// isHighSeverity checks if an incident has a severity label of "critical" or "high".
func isHighSeverity(incident *models.Incident) bool {
	if severity, ok := incident.Labels["severity"]; ok {
		return severity == "critical" || severity == "high"
	}
	return false
}

func NewBot(token string, service *service.IncidentService, userRepo service.UserRepository, suggester *service.ActionSuggester, alertChannelID int64) (*Bot, error) {
	pref := telebot.Settings{Token: token, Poller: &telebot.LongPoller{Timeout: 10 * time.Second}}
	b, err := telebot.NewBot(pref)
	if err != nil {
		return nil, err
	}
	botInstance := &Bot{
		bot:            b,
		service:        service,
		userRepo:       userRepo,
		suggester:      suggester,
		userStates:     make(map[int64]*userState),
		viewRegistry:   make(map[uint]map[string]telebot.Editable),
		alertChannelID: alertChannelID,
	}
	b.Use(botInstance.authMiddleware())
	return botInstance, nil
}

func (b *Bot) Start(notifChan, updateChan, topicDeletionChan <-chan *models.Incident) {
	b.registerHandlers()
	go b.startNotifier(notifChan)
	go b.startUpdateListener(updateChan)
	go b.startTopicDeletionListener(topicDeletionChan)
	log.Println("Telegram bot starting...")
	b.bot.Start()
}

func (b *Bot) startNotifier(notifChan <-chan *models.Incident) {
	log.Println("Notification listener started.")
	for incident := range notifChan {
		log.Printf("Received notification for new incident: %s", incident.Summary)

		if b.alertChannelID == 0 {
			log.Println("Alert channel ID is not configured, skipping notification.")
			continue
		}

		chat := &telebot.Chat{ID: b.alertChannelID}

		if isHighSeverity(incident) {
			b.handleHighSeverityIncident(chat, incident)
		} else {
			b.handleLowSeverityIncident(chat, incident)
		}
	}
}

func (b *Bot) handleHighSeverityIncident(chat *telebot.Chat, incident *models.Incident) {
	// 1. Create a topic for the high-severity incident.
	topic, err := b.bot.CreateTopic(chat, &telebot.Topic{Name: incident.Summary})
	if err != nil {
		log.Printf("Failed to create topic for incident %d: %v. Falling back to main channel.", incident.ID, err)
		b.handleLowSeverityIncident(chat, incident) // Fallback to standard notification.
		return
	}
	b.service.SetTelegramTopicID(context.Background(), incident.ID, int64(topic.ThreadID))

	// 2. Send the fully formatted message with keyboard directly to the topic.
	message := b.formatIncidentMessage(incident, false)
	keyboard := b.buildIncidentViewKeyboard(incident, false)
	topicSendOpts := &telebot.SendOptions{
		ThreadID:              topic.ThreadID,
		ParseMode:             telebot.ModeMarkdownV2,
		ReplyMarkup:           &telebot.ReplyMarkup{InlineKeyboard: keyboard},
		DisableWebPagePreview: true,
	}
	msg, err := b.bot.Send(chat, message, topicSendOpts)
	if err != nil {
		log.Printf("Failed to send notification to topic %d: %v", topic.ThreadID, err)
		return
	}

	// 3. Save the message ID for future updates and register the view.
	b.service.SetTelegramMessageID(context.Background(), incident.ID, msg.Chat.ID, int64(msg.ID))
	b.addIncidentView(incident.ID, msg)

	// 4. Send a separate, full-content message to the main channel with a button to jump to the topic.
	summaryMessage := b.formatIncidentMessage(incident, false)
	channelIDForLink := strings.TrimPrefix(strconv.FormatInt(b.alertChannelID, 10), "-100")
	topicURL := fmt.Sprintf("https://t.me/c/%s/%d", channelIDForLink, topic.ThreadID)
	linkKeyboard := [][]telebot.InlineButton{
		{{Text: "Перейти к обсуждению", URL: topicURL}},
	}
	summarySendOpts := &telebot.SendOptions{
		ParseMode:   telebot.ModeMarkdownV2,
		ReplyMarkup: &telebot.ReplyMarkup{InlineKeyboard: linkKeyboard},
	}
	summaryMsg, err := b.bot.Send(chat, summaryMessage, summarySendOpts)
	if err != nil {
		log.Printf("Failed to send summary notification to channel %d: %v", b.alertChannelID, err)
	} else {
		// Register the summary message as well, so it can be updated.
		b.addIncidentView(incident.ID, summaryMsg)
	}
}

func (b *Bot) startTopicDeletionListener(deletionChan <-chan *models.Incident) {
	log.Println("Topic deletion listener started.")
	for incident := range deletionChan {
		if !incident.TelegramChatID.Valid || !incident.TelegramTopicID.Valid {
			log.Printf("Cannot delete topic for incident %d: missing chat or topic ID.", incident.ID)
			continue
		}

		chat := &telebot.Chat{ID: incident.TelegramChatID.Int64}
		topic := &telebot.Topic{ThreadID: int(incident.TelegramTopicID.Int64)}

		err := b.bot.DeleteTopic(chat, topic)
		if err != nil {
			log.Printf("Failed to delete topic %d for incident %d: %v", topic.ThreadID, incident.ID, err)
		} else {
			log.Printf("Successfully deleted topic %d for incident %d.", topic.ThreadID, incident.ID)
			// После успешного удаления топика, можно убрать ID из инцидента в БД,
			// чтобы избежать повторных попыток удаления.
			b.service.SetTelegramTopicID(context.Background(), incident.ID, 0) // 0 или какое-то другое невалидное значение
		}
	}
}

func (b *Bot) handleLowSeverityIncident(chat *telebot.Chat, incident *models.Incident) {
	// Send the fully formatted message with keyboard directly to the main channel.
	message := b.formatIncidentMessage(incident, false)
	keyboard := b.buildIncidentViewKeyboard(incident, false)
	sendOpts := &telebot.SendOptions{
		ParseMode:             telebot.ModeMarkdownV2,
		ReplyMarkup:           &telebot.ReplyMarkup{InlineKeyboard: keyboard},
		DisableWebPagePreview: true,
	}
	msg, err := b.bot.Send(chat, message, sendOpts)
	if err != nil {
		log.Printf("Failed to send low-severity notification to channel %d: %v", b.alertChannelID, err)
		return
	}

	// Save the message ID for future updates and register the view.
	b.service.SetTelegramMessageID(context.Background(), incident.ID, msg.Chat.ID, int64(msg.ID))
	b.addIncidentView(incident.ID, msg)
}

func (b *Bot) startUpdateListener(updateChan <-chan *models.Incident) {
	log.Println("Update listener started.")
	for incident := range updateChan {
		log.Printf("Received update for incident ID %d", incident.ID)

		if !incident.TelegramChatID.Valid || !incident.TelegramMessageID.Valid {
			log.Printf("Incident %d does not have a Telegram message ID, skipping update.", incident.ID)
			continue
		}

		// It's better to fetch the latest incident state before rendering
		freshIncident, err := b.service.GetIncidentByID(context.Background(), incident.ID)
		if err != nil {
			log.Printf("Error fetching incident %d for update: %v", incident.ID, err)
			continue
		}

		b.updateIncidentView(freshIncident)

		if freshIncident.Status == models.StatusResolved || freshIncident.Status == models.StatusRejected {
			if freshIncident.TelegramTopicID.Valid {
				topic := &telebot.Topic{ThreadID: int(freshIncident.TelegramTopicID.Int64)}
				err := b.bot.CloseTopic(&telebot.Chat{ID: freshIncident.TelegramChatID.Int64}, topic)
				if err != nil {
					log.Printf("Failed to close topic %d for incident %d: %v", freshIncident.TelegramTopicID.Int64, freshIncident.ID, err)
				}
			}
		}
	}
}

func (b *Bot) registerHandlers() {
	b.bot.Handle("/start", b.handleStart)
	b.bot.Handle("/incidents", b.handleListIncidents)
	b.bot.Handle("/history", b.handleHistory)
	b.bot.Handle("/delete_incident_topic", b.handleDeleteIncidentTopic)
	b.bot.Handle(telebot.OnCallback, b.handleCallback)
	b.bot.Handle(telebot.OnText, b.handleTextMessage)
}

func (b *Bot) handleStart(c telebot.Context) error {
	return c.Send("Добро пожаловать! Используйте /incidents для просмотра активных инцидентов и /history для просмотра закрытых.")
}

func (b *Bot) handleListIncidents(c telebot.Context) error {
	incidents, err := b.service.ListActiveIncidents(c.Get("ctx").(context.Context))
	if err != nil {
		return c.Send("Не удалось получить список инцидентов.")
	}
	if len(incidents) == 0 {
		return c.Send("Активных инцидентов нет.")
	}
	var keyboard [][]telebot.InlineButton
	for _, inc := range incidents {
		row := []telebot.InlineButton{{
			Text: fmt.Sprintf("🚨 %s (%s)", inc.Summary, inc.Status),
			Data: showActionsPrefix + strconv.FormatUint(uint64(inc.ID), 10),
		}}
		keyboard = append(keyboard, row)
	}
	return c.Send("Активные инциденты:", &telebot.ReplyMarkup{InlineKeyboard: keyboard})
}

func (b *Bot) handleDeleteIncidentTopic(c telebot.Context) error {
	args := c.Args()
	if len(args) != 1 {
		return c.Send("Пожалуйста, укажите ID инцидента. \nИспользование: `/delete_incident_topic <ID>`")
	}

	incidentID, err := strconv.ParseUint(args[0], 10, 32)
	if err != nil {
		return c.Send("Неверный ID инцидента. Пожалуйста, введите число.")
	}

	incident, err := b.service.GetIncidentByID(c.Get("ctx").(context.Context), uint(incidentID))
	if err != nil {
		return c.Send(fmt.Sprintf("Инцидент с ID %d не найден.", incidentID))
	}

	if !incident.TelegramTopicID.Valid || incident.TelegramTopicID.Int64 == 0 {
		return c.Send(fmt.Sprintf("У инцидента #%d нет связанного топика для удаления.", incident.ID))
	}

	chat := &telebot.Chat{ID: incident.TelegramChatID.Int64}
	topic := &telebot.Topic{ThreadID: int(incident.TelegramTopicID.Int64)}

	err = b.bot.DeleteTopic(chat, topic)
	if err != nil {
		log.Printf("Failed to manually delete topic %d for incident %d: %v", topic.ThreadID, incident.ID, err)
		return c.Send(fmt.Sprintf("Не удалось удалить топик для инцидента #%d. Ошибка: %v", incident.ID, err))
	}

	log.Printf("Manually deleted topic %d for incident %d by user %s.", topic.ThreadID, incident.ID, c.Sender().Username)
	b.service.SetTelegramTopicID(context.Background(), incident.ID, 0)

	return c.Send(fmt.Sprintf("Топик для инцидента #%d успешно удален.", incident.ID))
}

func (b *Bot) handleHistory(c telebot.Context) error {
	incidents, err := b.service.ListClosed(c.Get("ctx").(context.Context), 10, 0)
	if err != nil {
		return c.Send("Не удалось получить историю инцидентов.")
	}
	if len(incidents) == 0 {
		return c.Send("История закрытых инцидентов пуста.")
	}
	var keyboard [][]telebot.InlineButton
	for _, inc := range incidents {
		icon := "✅"
		if inc.Status == models.StatusRejected {
			icon = "❌"
		}
		row := []telebot.InlineButton{{
			Text: fmt.Sprintf("%s %s (%s)", icon, inc.Summary, inc.Status),
			Data: viewIncidentPrefix + strconv.FormatUint(uint64(inc.ID), 10),
		}}
		keyboard = append(keyboard, row)
	}
	return c.Send("Последние закрытые инциденты:", &telebot.ReplyMarkup{InlineKeyboard: keyboard})
}

func (b *Bot) handleCallback(c telebot.Context) error {
	data := c.Data()
	parts := strings.Split(data, ":")
	if len(parts) < 2 {
		return c.Respond()
	}
	prefix := parts[0] + ":"
	incidentID, err := strconv.ParseUint(parts[1], 10, 32)
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "Invalid incident ID"})
	}

	switch prefix {
	case viewIncidentPrefix:
		return b.showIncidentView(c, uint(incidentID), false)
	case showActionsPrefix:
		return b.showActionsView(c, uint(incidentID), false)
	case closeIncidentPrefix:
		return b.showCloseOptions(c, uint(incidentID))
	case setStatusPrefix:
		return b.handleSetStatus(c)
	case performActionPrefix:
		return b.handlePerformAction(c)
	case viewResourcePrefix:
		return b.showResourceActionsView(c)
	case performResourceActionPrefix:
		return b.handlePerformResourceAction(c)
	case scaleDeploymentPrefix:
		return b.handleScaleDeployment(c)
	case allocateHardwarePrefix:
		return b.handleAllocateHardware(c)
	case toggleHistoryPrefix:
		return b.handleToggleHistory(c)
	default:
		return c.Respond()
	}
}

func (b *Bot) handleTextMessage(c telebot.Context) error {
	b.mu.Lock()
	state, ok := b.userStates[c.Sender().ID]
	if !ok {
		b.mu.Unlock()
		return nil
	}

	if state.AwaitingRejectReasonFor != 0 {
		incidentID := state.AwaitingRejectReasonFor
		state.AwaitingRejectReasonFor = 0
		b.mu.Unlock()

		reason := c.Text()
		user := c.Get("ctx").(context.Context).Value("user").(*models.User)

		err := b.service.UpdateStatus(c.Get("ctx").(context.Context), user.ID, incidentID, models.StatusRejected, reason)
		if err != nil {
			return c.Send("Не удалось обновить статус инцидента.")
		}
		sendOpts, _ := b.getSendOptionsForIncident(c.Get("ctx").(context.Context), incidentID)
		b.bot.Send(c.Chat(), "Инцидент отклонен. Спасибо за обратную связь!", sendOpts)
		return c.Delete()
	}

	if state.AwaitingReplicaCountFor != nil {
		inputState := state.AwaitingReplicaCountFor
		state.AwaitingReplicaCountFor = nil
		b.mu.Unlock()

		replicaCount, err := strconv.Atoi(c.Text())
		if err != nil || replicaCount < 0 {
			return c.Send("Неверное количество реплик. Пожалуйста, введите целое положительное число.")
		}

		req := inputState.Request
		req.Parameters["replicas"] = c.Text()
		result, err := b.service.ExecuteAction(c.Get("ctx").(context.Context), *req)
		sendOpts, _ := b.getSendOptionsForIncident(c.Get("ctx").(context.Context), req.IncidentID)
		if err != nil {
			b.bot.Send(c.Chat(), fmt.Sprintf("Ошибка: %v", err), sendOpts)
		} else {
			b.bot.Send(c.Chat(), result.Message, sendOpts)
		}

		c.Delete()
		return b.renderResourceActionsView(c, req.IncidentID, "deployment", req.Parameters["deployment"], &inputState.ChatID, &inputState.MessageID)
	}

	if state.AwaitingHardwareRequestFor != nil {
		inputState := state.AwaitingHardwareRequestFor
		state.AwaitingHardwareRequestFor = nil
		b.mu.Unlock()

		req := inputState.Request
		req.Parameters["resources"] = c.Text()
		result, err := b.service.ExecuteAction(c.Get("ctx").(context.Context), *req)
		sendOpts, _ := b.getSendOptionsForIncident(c.Get("ctx").(context.Context), req.IncidentID)
		if err != nil {
			b.bot.Send(c.Chat(), fmt.Sprintf("Ошибка: %v", err), sendOpts)
		} else {
			b.bot.Send(c.Chat(), result.Message, sendOpts)
		}

		c.Delete()
		return b.renderResourceActionsView(c, req.IncidentID, "pod", req.Parameters["pod"], &inputState.ChatID, &inputState.MessageID)
	}

	b.mu.Unlock()
	return nil
}

func (b *Bot) showIncidentView(c telebot.Context, incidentID uint, historyVisible bool) error {
	incident, err := b.service.GetIncidentByID(c.Get("ctx").(context.Context), incidentID)
	if err != nil {
		return c.EditOrSend("Не удалось найти инцидент.")
	}

	if incident.Status != models.StatusActive {
		return b.showClosedIncidentView(c, incident, historyVisible)
	}

	message := b.formatIncidentMessage(incident, historyVisible)
	keyboard := b.buildIncidentViewKeyboard(incident, historyVisible)
	err = c.Edit(message, &telebot.ReplyMarkup{InlineKeyboard: keyboard}, telebot.ModeMarkdownV2)
	if err == nil {
		b.addIncidentView(incident.ID, c.Message())
	}
	if err != nil && strings.Contains(err.Error(), "message is not modified") {
		return c.Respond()
	}
	return err
}

func (b *Bot) showActionsView(c telebot.Context, incidentID uint, historyVisible bool) error {
	incident, err := b.service.GetIncidentByID(c.Get("ctx").(context.Context), incidentID)
	if err != nil {
		return c.EditOrSend("Не удалось найти инцидент.")
	}
	message := b.formatIncidentMessage(incident, historyVisible)
	suggestedActions := b.suggester.SuggestActions(incident)
	keyboard := b.buildActionsViewKeyboard(incident, suggestedActions, historyVisible)
	err = c.Edit(message, &telebot.ReplyMarkup{InlineKeyboard: keyboard}, telebot.ModeMarkdownV2)
	if err == nil {
		b.addIncidentView(incident.ID, c.Message())
	}
	if err != nil && strings.Contains(err.Error(), "message is not modified") {
		return c.Respond()
	}
	return err
}

func (b *Bot) renderResourceActionsView(c telebot.Context, incidentID uint, resourceType, resourceName string, chatID *int64, messageID *int) error {
	ctx := c.Get("ctx").(context.Context)
	incident, err := b.service.GetIncidentByID(ctx, incidentID)
	if err != nil {
		return c.EditOrSend("Не удалось найти инцидент.")
	}

	detailsReq := models.ResourceDetailsRequest{
		IncidentID:   incidentID,
		ResourceType: resourceType,
		ResourceName: resourceName,
		Labels:       incident.Labels,
	}
	details, err := b.service.GetResourceDetails(ctx, detailsReq)

	var messageBuilder strings.Builder
	messageBuilder.WriteString(fmt.Sprintf("*Ресурс: %s `%s`*\n\n", strings.Title(resourceType), escapeMarkdown(resourceName)))

	if err != nil {
		log.Printf("Could not get resource details: %v", err)
		messageBuilder.WriteString("_Не удалось загрузить детали ресурса\\._\n\n")
	} else {
		messageBuilder.WriteString(fmt.Sprintf("∙ *Статус:* `%s`\n", escapeMarkdown(details.Status)))
		if details.ReplicasInfo != "" {
			messageBuilder.WriteString(fmt.Sprintf("∙ *Реплики:* `%s`\n", escapeMarkdown(details.ReplicasInfo)))
		}
		if details.Restarts != "" {
			messageBuilder.WriteString(fmt.Sprintf("∙ *Перезапуски:* `%s`\n", escapeMarkdown(details.Restarts)))
		}
		messageBuilder.WriteString(fmt.Sprintf("∙ *Возраст:* `%s`\n", escapeMarkdown(details.Age)))
		messageBuilder.WriteString("\n")
	}

	messageBuilder.WriteString("Выберите действие:")

	actions := b.suggester.SuggestActionsForResource(incident, resourceType, resourceName)
	keyboard := b.buildResourceActionsKeyboard(incident, resourceType, resourceName, actions)

	messageText := messageBuilder.String()
	replyMarkup := &telebot.ReplyMarkup{InlineKeyboard: keyboard}

	if messageID != nil && chatID != nil {
		editable := &telebot.StoredMessage{MessageID: strconv.Itoa(*messageID), ChatID: *chatID}
		_, err = b.bot.Edit(editable, messageText, replyMarkup, telebot.ModeMarkdownV2)
	} else {
		err = c.Edit(messageText, replyMarkup, telebot.ModeMarkdownV2)
	}

	if err != nil && strings.Contains(err.Error(), "message is not modified") {
		return c.Respond()
	}
	return err
}

func (b *Bot) showResourceActionsView(c telebot.Context) error {
	parts := strings.Split(c.Data(), ":")
	if len(parts) < 4 {
		log.Printf("Invalid callback data for showResourceActionsView: %s", c.Data())
		return c.Respond(&telebot.CallbackResponse{Text: "Invalid callback data"})
	}
	incidentID, _ := strconv.ParseUint(parts[1], 10, 32)
	resourceType := parts[2]
	resourceName := parts[3]

	return b.renderResourceActionsView(c, uint(incidentID), resourceType, resourceName, nil, nil)
}

func (b *Bot) showCloseOptions(c telebot.Context, incidentID uint) error {
	keyboard := b.buildCloseOptionsKeyboard(incidentID)
	return c.Edit("Выберите статус для закрытия инцидента:", &telebot.ReplyMarkup{InlineKeyboard: keyboard})
}

func (b *Bot) handleSetStatus(c telebot.Context) error {
	parts := strings.Split(c.Data(), ":")
	incidentID, _ := strconv.ParseUint(parts[1], 10, 32)
	status := models.IncidentStatus(parts[2])
	user := c.Get("ctx").(context.Context).Value("user").(*models.User)

	if status == models.StatusRejected {
		b.mu.Lock()
		b.userStates[c.Sender().ID] = &userState{AwaitingRejectReasonFor: uint(incidentID)}
		b.mu.Unlock()
		return c.Edit("Пожалуйста, введите причину отклонения инцидента одним сообщением.")
	}

	err := b.service.UpdateStatus(c.Get("ctx").(context.Context), user.ID, uint(incidentID), status, "")
	if err != nil {
		return c.Send("Не удалось обновить статус инцидента.")
	}
	sendOpts, _ := b.getSendOptionsForIncident(c.Get("ctx").(context.Context), uint(incidentID))
	b.bot.Send(c.Chat(), fmt.Sprintf("Статус инцидента обновлен на '%s'.", status), sendOpts)

	// Если инцидент закрыт, удаляем его из отслеживаемых
	if status == models.StatusResolved || status == models.StatusRejected {
		b.removeIncidentView(uint(incidentID))
		incident, err := b.service.GetIncidentByID(c.Get("ctx").(context.Context), uint(incidentID))
		if err == nil {
			return b.showClosedIncidentView(c, incident, false)
		}
	}

	return c.Delete()
}

func (b *Bot) handlePerformAction(c telebot.Context) error {
	parts := strings.Split(c.Data(), ":")
	incidentID, _ := strconv.ParseUint(parts[1], 10, 32)
	actionIndex, err := strconv.Atoi(parts[2])
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "Invalid action index"})
	}

	incident, err := b.service.GetIncidentByID(c.Get("ctx").(context.Context), uint(incidentID))
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "Incident not found"})
	}

	actions := b.suggester.SuggestActions(incident)
	if actionIndex < 0 || actionIndex >= len(actions) {
		return c.Respond(&telebot.CallbackResponse{Text: "Action no longer valid"})
	}
	action := actions[actionIndex]

	user := c.Get("ctx").(context.Context).Value("user").(*models.User)
	req := models.ActionRequest{
		Action:     action.Action,
		IncidentID: uint(incidentID),
		UserID:     user.ID,
		Parameters: action.Parameters,
	}

	result, err := b.service.ExecuteAction(c.Get("ctx").(context.Context), req)
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: fmt.Sprintf("Ошибка: %v", err)})
	}

	return b.handleActionResult(c, uint(incidentID), req, result)
}

func (b *Bot) handlePerformResourceAction(c telebot.Context) error {
	parts := strings.Split(c.Data(), ":")
	incidentID, _ := strconv.ParseUint(parts[1], 10, 32)
	resourceType := parts[2]
	resourceName := parts[3]
	actionIndex, err := strconv.Atoi(parts[4])
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "Invalid action index"})
	}

	incident, err := b.service.GetIncidentByID(c.Get("ctx").(context.Context), uint(incidentID))
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "Incident not found"})
	}

	actions := b.suggester.SuggestActionsForResource(incident, resourceType, resourceName)
	if actionIndex < 0 || actionIndex >= len(actions) {
		return c.Respond(&telebot.CallbackResponse{Text: "Action no longer valid"})
	}
	action := actions[actionIndex]

	user := c.Get("ctx").(context.Context).Value("user").(*models.User)
	req := models.ActionRequest{
		Action:     action.Action,
		IncidentID: uint(incidentID),
		UserID:     user.ID,
		Parameters: action.Parameters,
	}

	result, err := b.service.ExecuteAction(c.Get("ctx").(context.Context), req)
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: fmt.Sprintf("Ошибка: %v", err)})
	}

	return b.handleActionResult(c, uint(incidentID), req, result)
}

func (b *Bot) handleActionResult(c telebot.Context, incidentID uint, req models.ActionRequest, result models.ActionResult) error {
	actionType := models.ActionType(req.Action)
	if actionType == models.ActionGetPodLogs || actionType == models.ActionDescribePod || actionType == models.ActionListPodsForDeployment {
		c.Respond()
	} else {
		alertText := result.Message
		if result.Error != "" {
			alertText = result.Error
		}
		c.Respond(&telebot.CallbackResponse{Text: alertText, ShowAlert: true})
	}

	if result.Error != "" {
		return b.showIncidentView(c, incidentID, false)
	}

	switch actionType {
	case models.ActionGetPodLogs, models.ActionDescribePod, models.ActionDescribeDeployment:
		if len(result.Message) > 4096 {
			result.Message = result.Message[:4090] + "\n..."
		}
		formattedMessage := fmt.Sprintf("```\n%s\n```", result.Message)
		sendOpts, err := b.getSendOptionsForIncident(c.Get("ctx").(context.Context), incidentID)
		if err != nil {
			log.Printf("Could not get send options for incident %d: %v", incidentID, err)
			// Fallback to sending in the same chat without a specific topic
			b.bot.Send(c.Chat(), formattedMessage, telebot.ModeMarkdown)
			return nil
		}
		sendOpts.ParseMode = telebot.ModeMarkdown
		b.bot.Send(c.Chat(), formattedMessage, sendOpts)

	case models.ActionListPodsForDeployment:
		if result.ResultData != nil && len(result.ResultData.Items) > 0 {
			return b.showDynamicResourceList(c, incidentID, result)
		}
	}

	if req.Action == string(models.ActionScaleDeployment) || req.Action == string(models.ActionAllocateHardware) {
		return nil
	}

	var callbackData string
	if c.Callback() != nil {
		callbackData = c.Callback().Data
	}

	if strings.HasPrefix(callbackData, performResourceActionPrefix) {
		return b.showResourceActionsView(c)
	}

	return b.showActionsView(c, incidentID, false)
}

func (b *Bot) showDynamicResourceList(c telebot.Context, incidentID uint, result models.ActionResult) error {
	var keyboard [][]telebot.InlineButton
	for _, item := range result.ResultData.Items {
		statusIcon := "🟢"
		if item.Status != "Running" {
			statusIcon = "🔴"
		}
		callbackData := fmt.Sprintf("%s%d:%s:%s", viewResourcePrefix, incidentID, result.ResultData.ItemType, item.Name)
		btn := telebot.InlineButton{Text: fmt.Sprintf("%s %s (%s)", statusIcon, item.Name, item.Status), Data: callbackData}
		keyboard = append(keyboard, []telebot.InlineButton{btn})
	}

	incident, err := b.service.GetIncidentByID(c.Get("ctx").(context.Context), incidentID)
	if err != nil {
		return c.EditOrSend("Не удалось найти инцидент.")
	}

	keyboard = append(keyboard, []telebot.InlineButton{
		{Text: "⬅️ Назад", Data: showActionsPrefix + strconv.FormatUint(uint64(incidentID), 10)},
		{Text: "🏠 К инциденту", Data: viewIncidentPrefix + strconv.FormatUint(uint64(incidentID), 10)},
	})

	if incident.Status == models.StatusActive {
		keyboard = append(keyboard, []telebot.InlineButton{{Text: "✅ Закрыть инцидент", Data: closeIncidentPrefix + strconv.FormatUint(uint64(incidentID), 10)}})
	}

	return c.Edit(escapeMarkdown(result.Message), &telebot.ReplyMarkup{InlineKeyboard: keyboard}, telebot.ModeMarkdownV2)
}

func (b *Bot) getSendOptionsForIncident(ctx context.Context, incidentID uint) (*telebot.SendOptions, error) {
	incident, err := b.service.GetIncidentByID(ctx, incidentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get incident: %w", err)
	}

	opts := &telebot.SendOptions{}
	if incident.TelegramTopicID.Valid {
		opts.ThreadID = int(incident.TelegramTopicID.Int64)
	}

	return opts, nil
}

func (b *Bot) buildIncidentViewKeyboard(incident *models.Incident, historyVisible bool) [][]telebot.InlineButton {
	var keyboard [][]telebot.InlineButton

	if incident.Status == models.StatusActive {
		keyboard = append(keyboard, []telebot.InlineButton{
			{Text: "✅ Закрыть инцидент", Data: closeIncidentPrefix + strconv.FormatUint(uint64(incident.ID), 10)},
			{Text: "▶️ Выполнить действия", Data: showActionsPrefix + strconv.FormatUint(uint64(incident.ID), 10)},
		})
	}

	if len(incident.AuditLog) > 0 {
		historyButtonText := "📖 Показать историю"
		if historyVisible {
			historyButtonText = "📖 Скрыть историю"
		}
		keyboard = append(keyboard, []telebot.InlineButton{
			{Text: historyButtonText, Data: fmt.Sprintf("%s%d:%t:main", toggleHistoryPrefix, incident.ID, !historyVisible)},
		})
	}

	return keyboard
}

func (b *Bot) buildSummaryViewKeyboard(incident *models.Incident, historyVisible bool) [][]telebot.InlineButton {
	var keyboard [][]telebot.InlineButton

	if len(incident.AuditLog) > 0 {
		historyButtonText := "📖 Показать историю"
		if historyVisible {
			historyButtonText = "📖 Скрыть историю"
		}
		keyboard = append(keyboard, []telebot.InlineButton{
			{Text: historyButtonText, Data: fmt.Sprintf("%s%d:%t:summary", toggleHistoryPrefix, incident.ID, !historyVisible)},
		})
	}

	if incident.TelegramTopicID.Valid {
		channelIDForLink := strings.TrimPrefix(strconv.FormatInt(b.alertChannelID, 10), "-100")
		topicURL := fmt.Sprintf("https://t.me/c/%s/%d", channelIDForLink, incident.TelegramTopicID.Int64)
		keyboard = append(keyboard, []telebot.InlineButton{{Text: "Перейти к обсуждению", URL: topicURL}})
	}

	return keyboard
}

func (b *Bot) buildActionsViewKeyboard(incident *models.Incident, actions []models.SuggestedAction, historyVisible bool) [][]telebot.InlineButton {
	var keyboard [][]telebot.InlineButton
	var actionRow []telebot.InlineButton
	for i, action := range actions {
		callbackData := fmt.Sprintf("%s%d:%d", performActionPrefix, incident.ID, i)
		actionRow = append(actionRow, telebot.InlineButton{Text: action.HumanReadable, Data: callbackData})
	}
	if len(actionRow) > 0 {
		keyboard = append(keyboard, actionRow)
	}

	if len(incident.AffectedResources) > 0 {
		if deployment, ok := incident.AffectedResources["deployment"]; ok {
			callbackData := fmt.Sprintf("%s%d:%s:%s", viewResourcePrefix, incident.ID, "deployment", deployment)
			keyboard = append(keyboard, []telebot.InlineButton{{Text: "🗂️ Действия с Deployment", Data: callbackData}})
		}
	}

	keyboard = append(keyboard, []telebot.InlineButton{{Text: "⬅️ Назад", Data: viewIncidentPrefix + strconv.FormatUint(uint64(incident.ID), 10)}})

	if incident.Status == models.StatusActive {
		keyboard = append(keyboard, []telebot.InlineButton{{Text: "✅ Закрыть инцидент", Data: closeIncidentPrefix + strconv.FormatUint(uint64(incident.ID), 10)}})
	}

	if len(incident.AuditLog) > 0 {
		historyButtonText := "📖 Показать историю"
		if historyVisible {
			historyButtonText = "📖 Скрыть историю"
		}
		keyboard = append(keyboard, []telebot.InlineButton{
			{Text: historyButtonText, Data: fmt.Sprintf("%s%d:%t:actions", toggleHistoryPrefix, incident.ID, !historyVisible)},
		})
	}

	return keyboard
}

func (b *Bot) buildResourceActionsKeyboard(incident *models.Incident, resourceType, resourceName string, actions []models.SuggestedAction) [][]telebot.InlineButton {
	var keyboard [][]telebot.InlineButton
	incidentID := incident.ID
	for i, action := range actions {
		callbackData := fmt.Sprintf("%s%d:%s:%s:%d", performResourceActionPrefix, incidentID, resourceType, resourceName, i)
		keyboard = append(keyboard, []telebot.InlineButton{{Text: action.HumanReadable, Data: callbackData}})
	}

	if resourceType == "deployment" {
		namespace := incident.Labels["namespace"]
		callbackData := fmt.Sprintf("%s%d:%s:%s:%s", scaleDeploymentPrefix, incidentID, resourceType, resourceName, namespace)
		keyboard = append(keyboard, []telebot.InlineButton{{Text: "↔️ Масштабировать", Data: callbackData}})
	}

	if resourceType == "pod" {
		callbackData := fmt.Sprintf("%s%d:%s:%s", allocateHardwarePrefix, incidentID, resourceType, resourceName)
		keyboard = append(keyboard, []telebot.InlineButton{{Text: "⚙️ Выделить ресурсы", Data: callbackData}})
	}

	keyboard = append(keyboard, []telebot.InlineButton{
		{Text: "⬅️ Назад", Data: showActionsPrefix + strconv.FormatUint(uint64(incidentID), 10)},
		{Text: "🏠 К инциденту", Data: viewIncidentPrefix + strconv.FormatUint(uint64(incidentID), 10)},
	})

	if incident.Status == models.StatusActive {
		keyboard = append(keyboard, []telebot.InlineButton{{Text: "✅ Закрыть инцидент", Data: closeIncidentPrefix + strconv.FormatUint(uint64(incident.ID), 10)}})
	}

	return keyboard
}

func (b *Bot) buildCloseOptionsKeyboard(incidentID uint) [][]telebot.InlineButton {
	idStr := strconv.FormatUint(uint64(incidentID), 10)
	return [][]telebot.InlineButton{
		{
			{Text: "Решен", Data: setStatusPrefix + idStr + ":" + string(models.StatusResolved)},
			{Text: "Отклонен", Data: setStatusPrefix + idStr + ":" + string(models.StatusRejected)},
		},
		{{Text: "⬅️ Назад", Data: viewIncidentPrefix + idStr}},
	}
}

func (b *Bot) authMiddleware() telebot.MiddlewareFunc {
	return func(next telebot.HandlerFunc) telebot.HandlerFunc {
		return func(c telebot.Context) error {
			if c.Sender() == nil {
				return next(c)
			}
			user, err := b.userRepo.FindOrCreateByTelegramID(context.Background(), c.Sender().ID, c.Sender().Username, c.Sender().FirstName, c.Sender().LastName)
			if err != nil {
				log.Printf("Auth middleware error: %v", err)
				return c.Send("Произошла ошибка аутентификации.")
			}
			ctx := context.WithValue(context.Background(), "user", user)
			c.Set("ctx", ctx)
			return next(c)
		}
	}
}

func (b *Bot) formatIncidentMessage(incident *models.Incident, historyVisible bool) string {
	var builder strings.Builder

	// Header
	builder.WriteString(fmt.Sprintf("🚨 *Инцидент: %s* 🚨\n", escapeMarkdown(incident.Summary)))

	// Status and Severity
	severity := "N/A"
	if s, ok := incident.Labels["severity"]; ok {
		severity = s
	}
	builder.WriteString(fmt.Sprintf("*Статус:* `%s` \\| *Серьезность:* `%s`\n", incident.Status, severity))
	builder.WriteString("━━━━━━━━━━━━━━━\n")

	// Details
	builder.WriteString("*📋 Детали:*\n")
	builder.WriteString(fmt.Sprintf("∙ *Описание:* %s\n", escapeMarkdown(incident.Description)))
	if namespace, ok := incident.Labels["namespace"]; ok {
		builder.WriteString(fmt.Sprintf("∙ *Namespace:* `%s`\n", escapeMarkdown(namespace)))
	}
	builder.WriteString(fmt.Sprintf("∙ *Начало:* `%s`\n", incident.StartsAt.Format(time.RFC1123)))
	builder.WriteString("━━━━━━━━━━━━━━━\n")

	// Resources
	builder.WriteString("*🛠 Ресурсы:*\n")
	if deployment, ok := incident.AffectedResources["deployment"]; ok {
		builder.WriteString(fmt.Sprintf("∙ *Deployment:* `%s`\n", escapeMarkdown(deployment)))
	}
	if pod, ok := incident.AffectedResources["pod"]; ok {
		builder.WriteString(fmt.Sprintf("∙ *Pod:* `%s`\n", escapeMarkdown(pod)))
	}
	builder.WriteString("━━━━━━━━━━━━━━━\n")

	// Audit Log
	builder.WriteString("*📖 История действий:*\n")
	if len(incident.AuditLog) > 0 {
		if historyVisible {
			for _, entry := range incident.AuditLog {
				builder.WriteString(fmt.Sprintf(
					"`%s` \\- *%s* by *%s* \\- *%s*\n",
					entry.Timestamp.Format("15:04:05"),
					escapeMarkdown(entry.Action),
					escapeMarkdown(entry.User.Username),
					escapeMarkdown(entry.Result),
				))
				// Keep additional details for specific actions
				if entry.Action == "update_status" {
					if reason, ok := entry.Parameters["reason"]; ok && reason != "" {
						builder.WriteString(fmt.Sprintf("  *Причина:* %s\n", escapeMarkdown(reason)))
					}
				}
				if entry.Action == string(models.ActionScaleDeployment) {
					if replicas, ok := entry.Parameters["replicas"]; ok {
						builder.WriteString(fmt.Sprintf("  *Реплики:* `%s`\n", escapeMarkdown(replicas)))
					}
				}
				if entry.Action == string(models.ActionAllocateHardware) {
					if resources, ok := entry.Parameters["resources"]; ok {
						builder.WriteString(fmt.Sprintf("  *Ресурсы:* `%s`\n", escapeMarkdown(resources)))
					}
				}
			}
		} else {
			builder.WriteString(fmt.Sprintf("_История действий скрыта \\(%d записей\\)\\. Нажмите кнопку ниже, чтобы показать\\._\n", len(incident.AuditLog)))
		}
	} else {
		builder.WriteString("_Нет записей в истории\\._\n")
	}

	return builder.String()
}

func (b *Bot) handleScaleDeployment(c telebot.Context) error {
	parts := strings.Split(c.Data(), ":")
	incidentID, _ := strconv.ParseUint(parts[1], 10, 32)
	resourceName := parts[3]
	namespace := parts[4]

	user := c.Get("ctx").(context.Context).Value("user").(*models.User)

	req := &models.ActionRequest{
		Action:     string(models.ActionScaleDeployment),
		IncidentID: uint(incidentID),
		UserID:     user.ID,
		Parameters: map[string]string{
			"deployment": resourceName,
			"namespace":  namespace,
		},
	}

	err := c.Edit("Введите желаемое количество реплик:")
	if err != nil {
		return err
	}

	b.mu.Lock()
	if b.userStates[c.Sender().ID] == nil {
		b.userStates[c.Sender().ID] = &userState{}
	}
	b.userStates[c.Sender().ID].AwaitingReplicaCountFor = &awaitingInputState{
		Request:   req,
		MessageID: c.Message().ID,
		ChatID:    c.Chat().ID,
	}
	b.mu.Unlock()

	return nil
}

func (b *Bot) handleAllocateHardware(c telebot.Context) error {
	parts := strings.Split(c.Data(), ":")
	incidentID, _ := strconv.ParseUint(parts[1], 10, 32)
	resourceName := parts[3]

	user := c.Get("ctx").(context.Context).Value("user").(*models.User)

	req := &models.ActionRequest{
		Action:     string(models.ActionAllocateHardware),
		IncidentID: uint(incidentID),
		UserID:     user.ID,
		Parameters: map[string]string{
			"pod": resourceName,
		},
	}

	err := c.Edit("Введите запрашиваемые ресурсы в формате `cpu=1.5, memory=512Mi`:")
	if err != nil {
		return err
	}

	b.mu.Lock()
	if b.userStates[c.Sender().ID] == nil {
		b.userStates[c.Sender().ID] = &userState{}
	}
	b.userStates[c.Sender().ID].AwaitingHardwareRequestFor = &awaitingInputState{
		Request:   req,
		MessageID: c.Message().ID,
		ChatID:    c.Chat().ID,
	}
	b.mu.Unlock()

	return nil
}

func escapeMarkdown(s string) string {
	replacer := strings.NewReplacer(
		"_", "\\_", "*", "\\*", "[", "\\[", "]", "\\]", "(",
		"\\(", ")", "\\)", "~", "\\~", "`", "\\`", ">", "\\>",
		"#", "\\#", "+", "\\+", "-", "\\-", "=", "\\=", "|",
		"\\|", "{", "\\{", "}", "\\}", ".", "\\.", "!", "\\!",
	)
	return replacer.Replace(s)
}

func (b *Bot) addIncidentView(incidentID uint, editable telebot.Editable) {
	b.registryMu.Lock()
	defer b.registryMu.Unlock()
	if _, ok := b.viewRegistry[incidentID]; !ok {
		b.viewRegistry[incidentID] = make(map[string]telebot.Editable)
	}
	key := getViewRegistryKey(editable)
	b.viewRegistry[incidentID][key] = editable
	log.Printf("Added view for incident %d. Total views for this incident: %d", incidentID, len(b.viewRegistry[incidentID]))
}

func (b *Bot) removeIncidentView(incidentID uint) {
	b.registryMu.Lock()
	defer b.registryMu.Unlock()
	delete(b.viewRegistry, incidentID)
	log.Printf("Removed all views for incident %d", incidentID)
}

func (b *Bot) updateIncidentView(incident *models.Incident) {
	b.registryMu.RLock()
	views, ok := b.viewRegistry[incident.ID]
	b.registryMu.RUnlock()

	if !ok {
		log.Printf("No views registered for incident %d, cannot update.", incident.ID)
		return
	}

	// For simplicity, we assume history is not visible on automatic updates.
	historyVisible := false
	message := b.formatIncidentMessage(incident, historyVisible)

	log.Printf("Attempting to update %d views for incident %d", len(views), incident.ID)
	for key, editable := range views {
		var keyboard [][]telebot.InlineButton
		msgSig, _ := editable.MessageSig()

		// The main message in the topic gets the full keyboard.
		// The summary message in the general channel gets a limited keyboard.
		if incident.TelegramMessageID.Valid && msgSig == strconv.FormatInt(incident.TelegramMessageID.Int64, 10) {
			keyboard = b.buildIncidentViewKeyboard(incident, historyVisible)
		} else if isHighSeverity(incident) {
			// This is a summary message for a high-severity incident.
			keyboard = b.buildSummaryViewKeyboard(incident, historyVisible)
		} else {
			// This is a low-severity incident message.
			keyboard = b.buildIncidentViewKeyboard(incident, historyVisible)
		}

		_, err := b.bot.Edit(editable, message, &telebot.ReplyMarkup{InlineKeyboard: keyboard}, telebot.ModeMarkdownV2)
		if err != nil {
			if strings.Contains(err.Error(), "message is not modified") {
				// Not an error, just no change needed.
			} else if strings.Contains(err.Error(), "message to edit not found") {
				log.Printf("View %s for incident %d not found, cannot update.", key, incident.ID)
			} else {
				log.Printf("Failed to update view %s for incident %d: %v", key, incident.ID, err)
			}
		} else {
			log.Printf("Successfully updated view %s for incident %d", key, incident.ID)
		}
	}
}

func getViewRegistryKey(editable telebot.Editable) string {
	msgSig, chatID := editable.MessageSig()
	return fmt.Sprintf("%d-%s", chatID, msgSig)
}

func (b *Bot) handleToggleHistory(c telebot.Context) error {
	parts := strings.Split(c.Data(), ":")
	incidentID, _ := strconv.ParseUint(parts[1], 10, 32)
	historyVisible, _ := strconv.ParseBool(parts[2])
	viewType := parts[3] // "main" or "actions"

	if viewType == "actions" {
		return b.showActionsView(c, uint(incidentID), historyVisible)
	}
	if viewType == "summary" {
		// This is a summary message, so we need to show the summary view.
		incident, err := b.service.GetIncidentByID(c.Get("ctx").(context.Context), uint(incidentID))
		if err != nil {
			return c.EditOrSend("Не удалось найти инцидент.")
		}
		message := b.formatIncidentMessage(incident, historyVisible)
		keyboard := b.buildSummaryViewKeyboard(incident, historyVisible)
		return c.Edit(message, &telebot.ReplyMarkup{InlineKeyboard: keyboard}, telebot.ModeMarkdownV2)
	}
	return b.showIncidentView(c, uint(incidentID), historyVisible)
}

func (b *Bot) showClosedIncidentView(c telebot.Context, incident *models.Incident, historyVisible bool) error {
	message := b.formatIncidentMessage(incident, historyVisible)
	var keyboard [][]telebot.InlineButton

	historyButtonText := "📖 Показать историю"
	if historyVisible {
		historyButtonText = "📖 Скрыть историю"
	}
	if isHighSeverity(incident) {
		keyboard = b.buildSummaryViewKeyboard(incident, historyVisible)
	} else {
		keyboard = append(keyboard, []telebot.InlineButton{
			{Text: historyButtonText, Data: fmt.Sprintf("%s%d:%t:closed", toggleHistoryPrefix, incident.ID, !historyVisible)},
		})
	}

	return c.Edit(message, &telebot.ReplyMarkup{InlineKeyboard: keyboard}, telebot.ModeMarkdownV2)
}
