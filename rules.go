package zero

import (
	log "github.com/sirupsen/logrus"
	"hash/crc64"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/wdvxdr1123/ZeroBot/message"
	"github.com/wdvxdr1123/ZeroBot/utils/helper"
)

const (
	KEY_PATTERN = "pattern_matched"
)

// Type check the ctx.Event's type
func Type(type_ string) Rule {
	t := strings.SplitN(type_, "/", 3)
	return func(ctx *Ctx) bool {
		if len(t) > 0 && t[0] != ctx.Event.PostType {
			return false
		}
		if len(t) > 1 && t[1] != ctx.Event.DetailType {
			return false
		}
		if len(t) > 2 && t[2] != ctx.Event.SubType {
			return false
		}
		return true
	}
}

// PrefixRule check if the message has the prefix and trim the prefix
//
// 检查消息前缀
func PrefixRule(prefixes ...string) Rule {
	return func(ctx *Ctx) bool {
		if len(ctx.Event.Message) == 0 || ctx.Event.Message[0].Type != "text" { // 确保无空指针
			return false
		}
		first := ctx.Event.Message[0]
		firstMessage := first.Data["text"]
		for _, prefix := range prefixes {
			if strings.HasPrefix(firstMessage, prefix) {
				ctx.State["prefix"] = prefix
				arg := strings.TrimLeft(firstMessage[len(prefix):], " ")
				if len(ctx.Event.Message) > 1 {
					arg += ctx.Event.Message[1:].ExtractPlainText()
				}
				ctx.State["args"] = arg
				return true
			}
		}
		return false
	}
}

// SuffixRule check if the message has the suffix and trim the suffix
//
// 检查消息后缀
func SuffixRule(suffixes ...string) Rule {
	return func(ctx *Ctx) bool {
		mLen := len(ctx.Event.Message)
		if mLen <= 0 { // 确保无空指针
			return false
		}
		last := ctx.Event.Message[mLen-1]
		if last.Type != "text" {
			return false
		}
		lastMessage := last.Data["text"]
		for _, suffix := range suffixes {
			if strings.HasSuffix(lastMessage, suffix) {
				ctx.State["suffix"] = suffix
				arg := strings.TrimRight(lastMessage[:len(lastMessage)-len(suffix)], " ")
				if mLen >= 2 {
					arg += ctx.Event.Message[:mLen].ExtractPlainText()
				}
				ctx.State["args"] = arg
				return true
			}
		}
		return false
	}
}

// CommandRule check if the message is a command and trim the command name
func CommandRule(commands ...string) Rule {
	return func(ctx *Ctx) bool {
		if len(ctx.Event.Message) == 0 {
			return false
		}
		firstMessage := strings.TrimSpace(ctx.ExtractPlainText())
		if !strings.HasPrefix(firstMessage, BotConfig.CommandPrefix) {
			return false
		}
		cmdMessage := firstMessage[len(BotConfig.CommandPrefix):]
		for _, command := range commands {
			if strings.HasPrefix(cmdMessage, command+" ") || strings.TrimSpace(cmdMessage) == command {
				ctx.State["command"] = command
				if strings.TrimSpace(cmdMessage) == command {
					ctx.State["args"] = ""
					log.Debugf("[matcher.CommandRule] triggered %s, args ", command)
					return true
				}
				arg := strings.TrimLeft(cmdMessage[len(command)+1:], " ")
				if len(ctx.Event.Message) > 1 {
					arg += ctx.Event.Message[1:].ExtractPlainText()
				}
				ctx.State["args"] = arg
				log.Debugf("[matcher.CommandRule] triggered %s, arg %s", command, arg)
				return true
			}
		}
		return false
	}
}

// RegexRule check if the message can be matched by the regex pattern
func RegexRule(regexPattern string) Rule {
	regex := regexp.MustCompile(regexPattern)
	return func(ctx *Ctx) bool {
		msg := ctx.MessageString()
		if matched := regex.FindStringSubmatch(msg); matched != nil {
			ctx.State["regex_matched"] = matched
			return true
		}
		return false
	}
}

type PatternSegment struct {
	Type     string
	Optional bool
	Matcher  func(ctx *Ctx, msg message.MessageSegment) bool
}

// SetOptional set Pattern is optional, is v is empty, Optional will be true
func (p *PatternSegment) SetOptional(v ...bool) *PatternSegment {
	if len(v) == 1 {
		p.Optional = v[0]
	} else {
		p.Optional = true
	}
	return p
}

type Pattern []PatternSegment

// PatternMatched save PatternRule context
type PatternMatched map[string]interface{}

func (p PatternMatched) AsText() PatternTextMatched {
	if _, ok := p["groups"]; !ok {
		return PatternTextMatched{}
	}
	return PatternTextMatched{HasValue: true, Groups: p["groups"].([]string)}
}
func (p PatternMatched) AsAt() PatternAtMatched {
	if _, ok := p["qq"]; !ok {
		return PatternAtMatched{}
	}
	return PatternAtMatched{HasValue: true, UID: p["qq"].(int64)}
}
func (p PatternMatched) AsImage() PatternImageMatched {
	if _, ok := p["file"]; !ok {
		return PatternImageMatched{}
	}
	return PatternImageMatched{HasValue: true, File: p["file"].(string)}
}
func (p PatternMatched) AsReply() PatternReplyMatched {
	if _, ok := p["id"]; !ok {
		return PatternReplyMatched{}
	}
	return PatternReplyMatched{HasValue: true, MessageID: p["id"].(string)}
}

// PatternImageMatched
// HasValue: false if not matched in optional Pattern
type PatternImageMatched = struct {
	HasValue bool
	File     string
}

// PatternAtMatched
// HasValue: false if not matched in optional Pattern
type PatternAtMatched = struct {
	HasValue bool
	UID      int64
}

// PatternTextMatched
// HasValue: false if not matched in optional Pattern
type PatternTextMatched = struct {
	HasValue bool
	Groups   []string
}

// PatternReplyMatched
// HasValue: false if not matched in optional Pattern
type PatternReplyMatched = struct {
	HasValue  bool
	MessageID string
}

// PatternText type zero.PatternTextMatched
func PatternText(regex string) *PatternSegment {
	re := regexp.MustCompile(regex)
	return &PatternSegment{
		Type: "text",
		Matcher: func(ctx *Ctx, msg message.MessageSegment) bool {
			s := msg.Data["text"]
			s = strings.Trim(s, " \n\r\t")
			matchString := re.MatchString(s)
			if matchString {
				if _, ok := ctx.State[KEY_PATTERN]; !ok {
					ctx.State[KEY_PATTERN] = make([]PatternMatched, 0, 1)
				}

				ctx.State[KEY_PATTERN] = append(ctx.State[KEY_PATTERN].([]PatternMatched),
					PatternMatched{
						"groups": re.FindStringSubmatch(s),
					})
			}
			return matchString
		},
	}
}

// PatternAt type zero.PatternAtMatched
func PatternAt() *PatternSegment {
	return &PatternSegment{
		Type: "at",
		Matcher: func(ctx *Ctx, msg message.MessageSegment) bool {
			if _, ok := ctx.State[KEY_PATTERN]; !ok {
				ctx.State[KEY_PATTERN] = make([]PatternMatched, 0, 1)
			}
			qq, _ := strconv.ParseInt(msg.Data["qq"], 10, 64)
			ctx.State[KEY_PATTERN] = append(ctx.State[KEY_PATTERN].([]PatternMatched),
				PatternMatched{
					"qq": qq,
				})
			return true
		},
	}
}

// PatternImage type zero.PatternImageMatched
func PatternImage() *PatternSegment {
	return &PatternSegment{
		Type: "image",
		Matcher: func(ctx *Ctx, msg message.MessageSegment) bool {
			if _, ok := ctx.State[KEY_PATTERN]; !ok {
				ctx.State[KEY_PATTERN] = make([]PatternMatched, 0, 1)
			}
			ctx.State[KEY_PATTERN] = append(ctx.State[KEY_PATTERN].([]PatternMatched), PatternMatched{
				"file": msg.Data["file"],
			})
			return true
		},
	}
}

// PatternReply type zero.PatternReplyMatched
func PatternReply() *PatternSegment {
	return &PatternSegment{
		Type: "reply",
		Matcher: func(ctx *Ctx, msg message.MessageSegment) bool {
			if _, ok := ctx.State[KEY_PATTERN]; !ok {
				ctx.State[KEY_PATTERN] = make([]PatternMatched, 0, 1)
			}
			ctx.State[KEY_PATTERN] = append(ctx.State[KEY_PATTERN].([]PatternMatched), PatternMatched{
				"id": msg.Data["id"],
			})
			return true
		},
	}
}
func containsOptional(pattern []*PatternSegment) bool {
	for _, p := range pattern {
		if p.Optional {
			return true
		}
	}
	return false
}
func patternMatch(ctx *Ctx, pattern []*PatternSegment, msgs []message.MessageSegment) bool {
	if !containsOptional(pattern) && len(pattern) != len(msgs) {
		return false
	}
	i := 0
	j := 0
	for i < len(pattern) && j < len(msgs) {
		if pattern[i].Type != (msgs[j].Type) || !pattern[i].Matcher(ctx, msgs[j]) {
			if pattern[i].Optional {
				if _, ok := ctx.State[KEY_PATTERN]; !ok {
					ctx.State[KEY_PATTERN] = make([]PatternMatched, 0, 1)
				}
				ctx.State[KEY_PATTERN] = append(ctx.State[KEY_PATTERN].([]PatternMatched), PatternMatched{})
				i++
				continue
			}
			return false
		}
		i++
		j++
	}
	return true
}

// PatternRule check if the message can be matched by the pattern
func PatternRule(pattern ...*PatternSegment) Rule {
	return func(ctx *Ctx) bool {
		// copy messages
		msgs := make([]message.MessageSegment, 0, len(ctx.Event.Message))
		msgs = append(msgs, ctx.Event.Message[0])
		for i := 1; i < len(ctx.Event.Message); i++ {
			if ctx.Event.Message[i-1].Type == "reply" && ctx.Event.Message[i].Type == "at" {
				// [reply][at]
				reply := ctx.GetMessage(ctx.Event.Message[i-1].Data["id"])
				if reply.MessageId.ID() == 0 || reply.Sender == nil || reply.Sender.ID == 0 {
					// failed to get history message
					msgs = append(msgs, ctx.Event.Message[i])
					continue
				}
				if strconv.FormatInt(reply.Sender.ID, 10) != ctx.Event.Message[i].Data["qq"] {
					// @ other user in reply
					msgs = append(msgs, ctx.Event.Message[i])
				}
			} else {
				msgs = append(msgs, ctx.Event.Message[i])
			}
		}
		return patternMatch(ctx, pattern, msgs)
	}
}

// ReplyRule check if the message is replying some message
func ReplyRule(messageID int64) Rule {
	return func(ctx *Ctx) bool {
		if len(ctx.Event.Message) == 0 {
			return false
		}
		if ctx.Event.Message[0].Type != "reply" {
			return false
		}
		if id, err := strconv.ParseInt(ctx.Event.Message[0].Data["id"], 10, 64); err == nil {
			return id == messageID
		}
		c := crc64.New(crc64.MakeTable(crc64.ISO))
		c.Write(helper.StringToBytes(ctx.Event.Message[0].Data["id"]))
		return int64(c.Sum64()) == messageID
	}
}

// KeywordRule check if the message has a keyword or keywords
func KeywordRule(src ...string) Rule {
	return func(ctx *Ctx) bool {
		msg := ctx.MessageString()
		for _, str := range src {
			if strings.Contains(msg, str) {
				ctx.State["keyword"] = str
				return true
			}
		}
		return false
	}
}

// FullMatchRule check if src has the same copy of the message
func FullMatchRule(src ...string) Rule {
	return func(ctx *Ctx) bool {
		msg := ctx.MessageString()
		for _, str := range src {
			if str == msg {
				ctx.State["matched"] = msg
				return true
			}
		}
		return false
	}
}

// OnlyToMe only triggered in conditions of @bot or begin with the nicknames
func OnlyToMe(ctx *Ctx) bool {
	return ctx.Event.IsToMe
}

// CheckUser only triggered by specific person
func CheckUser(userId ...int64) Rule {
	return func(ctx *Ctx) bool {
		for _, uid := range userId {
			if ctx.Event.UserID == uid {
				return true
			}
		}
		return false
	}
}

// CheckGroup only triggered in specific group
func CheckGroup(grpId ...int64) Rule {
	return func(ctx *Ctx) bool {
		for _, gid := range grpId {
			if ctx.Event.GroupID == gid {
				return true
			}
		}
		return false
	}
}

func CheckArgs(pred func(ctx *Ctx, args []string) bool) Rule {
	return func(ctx *Ctx) bool {
		if len(ctx.State["args"].(string)) == 0 {
			return pred(ctx, make([]string, 0))
		}
		return pred(ctx, ParseShell(ctx.State["args"].(string)))
	}
}

// OnlyPrivate requires that the ctx.Event is private message
func OnlyPrivate(ctx *Ctx) bool {
	return ctx.Event.PostType == "message" && ctx.Event.DetailType == "private"
}

// OnlyPublic requires that the ctx.Event is public/group or public/guild message
func OnlyPublic(ctx *Ctx) bool {
	return ctx.Event.PostType == "message" && (ctx.Event.DetailType == "group" || ctx.Event.DetailType == "guild")
}

// OnlyGroup requires that the ctx.Event is public/group message
func OnlyGroup(ctx *Ctx) bool {
	return ctx.Event.PostType == "message" && ctx.Event.DetailType == "group"
}

// OnlyGuild requires that the ctx.Event is public/guild message
func OnlyGuild(ctx *Ctx) bool {
	return ctx.Event.PostType == "message" && ctx.Event.DetailType == "guild"
}

func issu(id int64) bool {
	for _, su := range BotConfig.SuperUsers {
		if su == id {
			return true
		}
	}
	return false
}

// SuperUserPermission only triggered by the bot's owner
func SuperUserPermission(ctx *Ctx) bool {
	return issu(ctx.Event.UserID)
}

// AdminPermission only triggered by the group admins or higher permission
func AdminPermission(ctx *Ctx) bool {
	return SuperUserPermission(ctx) || ctx.Event.Sender.Role == "owner" || ctx.Event.Sender.Role == "admin"
}

// OwnerPermission only triggered by the group owner or higher permission
func OwnerPermission(ctx *Ctx) bool {
	return SuperUserPermission(ctx) || ctx.Event.Sender.Role == "owner"
}

// UserOrGrpAdmin 允许用户单独使用或群管使用
func UserOrGrpAdmin(ctx *Ctx) bool {
	if OnlyGroup(ctx) {
		return AdminPermission(ctx)
	}
	return OnlyToMe(ctx)
}

// GroupHigherPermission 群发送者权限高于 target
//
// 隐含 OnlyGroup 判断
func GroupHigherPermission(gettarget func(ctx *Ctx) int64) Rule {
	return func(ctx *Ctx) bool {
		if !OnlyGroup(ctx) {
			return false
		}
		target := gettarget(ctx)
		if target == ctx.Event.UserID { // 特判, 自己和自己比
			return false
		}
		if SuperUserPermission(ctx) {
			sender := ctx.Event.UserID
			return BotConfig.GetFirstSuperUser(sender, target) == sender
		}
		if ctx.Event.Sender.Role == "owner" {
			return !issu(target) && ctx.GetThisGroupMemberInfo(target, false).Get("role").Str != "owner"
		}
		if ctx.Event.Sender.Role == "admin" {
			tgtrole := ctx.GetThisGroupMemberInfo(target, false).Get("role").Str
			return !issu(target) && tgtrole != "owner" && tgtrole != "admin"
		}
		return false // member is the lowest
	}
}

// HasPicture 消息含有图片返回 true
func HasPicture(ctx *Ctx) bool {
	var urls = []string{}
	for _, elem := range ctx.Event.Message {
		if elem.Type == "image" {
			if elem.Data["url"] != "" {
				urls = append(urls, elem.Data["url"])
			}
		}
	}
	if len(urls) > 0 {
		ctx.State["image_url"] = urls
		return true
	}
	return false
}

// MustProvidePicture 消息不存在图片阻塞120秒至有图片，超时返回 false
func MustProvidePicture(ctx *Ctx) bool {
	if HasPicture(ctx) {
		return true
	}
	// 没有图片就索取
	ctx.SendChain(message.Text("请发送一张图片"))
	next := NewFutureEvent("message", 999, true, ctx.CheckSession(), HasPicture).Next()
	select {
	case <-time.After(time.Second * 120):
		return false
	case newCtx := <-next:
		ctx.State["image_url"] = newCtx.State["image_url"]
		ctx.Event.MessageID = newCtx.Event.MessageID
		return true
	}
}
