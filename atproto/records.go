package atproto

// AllRecordTypes contains all known app.bsky record types, including methods and definitions
var AllRecordTypes = []string{
	// Actor
	"app.bsky.actor.profile",
	"app.bsky.actor.defs",

	// Feed
	"app.bsky.feed.generator",
	"app.bsky.feed.like",
	"app.bsky.feed.post",
	"app.bsky.feed.repost",
	"app.bsky.feed.threadgate",
	"app.bsky.feed.defs",
	"app.bsky.feed.describeFeedGenerator",
	"app.bsky.feed.getActorFeeds",
	"app.bsky.feed.getActorLikes",
	"app.bsky.feed.getAuthorFeed",
	"app.bsky.feed.getFeed",
	"app.bsky.feed.getFeedGenerator",
	"app.bsky.feed.getFeedGenerators",
	"app.bsky.feed.getFeedSkeleton",
	"app.bsky.feed.getLikes",
	"app.bsky.feed.getListFeed",
	"app.bsky.feed.getPostThread",
	"app.bsky.feed.getPosts",
	"app.bsky.feed.getRepostedBy",
	"app.bsky.feed.getSuggestedFeeds",
	"app.bsky.feed.getTimeline",
	"app.bsky.feed.searchPosts",

	// Graph
	"app.bsky.graph.block",
	"app.bsky.graph.follow",
	"app.bsky.graph.list",
	"app.bsky.graph.listitem",
	"app.bsky.graph.listblock",
	"app.bsky.graph.defs",
	"app.bsky.graph.getBlocks",
	"app.bsky.graph.getFollowers",
	"app.bsky.graph.getFollows",
	"app.bsky.graph.getList",
	"app.bsky.graph.getListBlocks",
	"app.bsky.graph.getListMutes",
	"app.bsky.graph.getLists",
	"app.bsky.graph.getMutes",
	"app.bsky.graph.getMutedLists",
	"app.bsky.graph.getRelationships",
	"app.bsky.graph.getSuggestedFollowsByActor",
	"app.bsky.graph.muteActor",
	"app.bsky.graph.muteActorList",
	"app.bsky.graph.unmuteActor",
	"app.bsky.graph.unmuteActorList",

	// Notification
	"app.bsky.notification.getUnreadCount",
	"app.bsky.notification.listNotifications",
	"app.bsky.notification.registerPush",
	"app.bsky.notification.updateSeen",

	// Unspecced
	"app.bsky.unspecced.getPopular",
	"app.bsky.unspecced.getPopularFeedGenerators",
	"app.bsky.unspecced.getTimelineSkeleton",
	"app.bsky.unspecced.searchActorsSkeleton",
	"app.bsky.unspecced.searchPostsSkeleton",

	// Labeler
	"app.bsky.labeler.service",
	"app.bsky.labeler.label",
	"app.bsky.labeler.getServices",

	// Richtext
	"app.bsky.richtext.facet",

	// Embed
	"app.bsky.embed.images",
	"app.bsky.embed.external",
	"app.bsky.embed.record",
	"app.bsky.embed.recordWithMedia",
}

// JetstreamRecordTypes contains only the record types that can appear in the firehose
var JetstreamRecordTypes = []string{
	"app.bsky.actor.profile",
	"app.bsky.feed.generator",
	"app.bsky.feed.like",
	"app.bsky.feed.post",
	"app.bsky.feed.repost",
	"app.bsky.feed.threadgate",
	"app.bsky.graph.block",
	"app.bsky.graph.follow",
	"app.bsky.graph.list",
	"app.bsky.graph.listitem",
	"app.bsky.graph.listblock",
	"app.bsky.labeler.service",
	"app.bsky.labeler.label",
}
