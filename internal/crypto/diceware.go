package crypto

import (
	"crypto/rand"
	"math/big"
	"strings"
)

// wordList is a curated list of easy-to-type, unambiguous English words.
// Kept short enough to embed, large enough for good entropy (~11 bits per word).
var wordList = []string{
	"able", "acid", "aged", "also", "arch", "area", "army", "atom",
	"aunt", "away", "back", "bake", "band", "bank", "barn", "base",
	"bath", "beam", "bear", "beat", "been", "bell", "belt", "bend",
	"bike", "bird", "bite", "blow", "blue", "blur", "boat", "body",
	"bold", "bolt", "bomb", "bond", "bone", "book", "boot", "born",
	"boss", "both", "bowl", "bulk", "bump", "burn", "bush", "busy",
	"buzz", "cage", "cake", "calm", "came", "camp", "cape", "card",
	"care", "cart", "case", "cash", "cast", "cave", "cell", "chat",
	"chef", "chin", "chip", "chop", "city", "clad", "clam", "clan",
	"clap", "claw", "clay", "clip", "club", "clue", "coal", "coat",
	"code", "coil", "coin", "cold", "colt", "come", "cone", "cook",
	"cool", "cope", "copy", "cord", "core", "corn", "cost", "cozy",
	"crew", "crop", "crow", "cube", "cult", "cups", "cure", "curl",
	"cute", "dale", "dame", "damp", "dare", "dark", "dart", "dash",
	"data", "dawn", "days", "deal", "dear", "deck", "deed", "deem",
	"deep", "deer", "demo", "deny", "desk", "dial", "dice", "diet",
	"dime", "dine", "dirt", "dish", "disk", "dock", "does", "dome",
	"done", "door", "dose", "dove", "down", "drag", "draw", "drip",
	"drop", "drum", "dual", "duck", "dude", "duke", "dull", "dump",
	"dune", "dusk", "dust", "duty", "each", "earl", "earn", "ease",
	"east", "easy", "echo", "edge", "edit", "else", "emit", "ends",
	"epic", "even", "ever", "evil", "exam", "exit", "expo", "face",
	"fact", "fade", "fail", "fair", "fake", "fall", "fame", "fang",
	"farm", "fast", "fate", "fawn", "fear", "feat", "feed", "feel",
	"feet", "fell", "felt", "fern", "fest", "file", "fill", "film",
	"find", "fine", "fire", "firm", "fish", "fist", "five", "flag",
	"flat", "flaw", "fled", "flew", "flex", "flip", "flock", "flow",
	"foam", "fold", "folk", "fond", "font", "food", "fool", "foot",
	"ford", "fork", "form", "fort", "foul", "four", "fowl", "free",
	"from", "fuel", "full", "fund", "fuse", "fury", "fuzz", "gain",
	"gait", "gale", "game", "gang", "gate", "gave", "gaze", "gear",
	"gene", "gift", "girl", "give", "glad", "glow", "glue", "goat",
	"goes", "gold", "golf", "gone", "good", "gown", "grab", "gram",
	"gray", "grew", "grid", "grim", "grin", "grip", "grow", "gulf",
	"gust", "hack", "hail", "hair", "half", "hall", "halt", "hand",
	"hang", "hard", "hare", "harm", "harp", "hash", "hate", "haul",
	"have", "hawk", "haze", "head", "heal", "heap", "hear", "heat",
	"heel", "held", "helm", "help", "herb", "herd", "here", "hero",
	"hide", "high", "hike", "hill", "hint", "hire", "hold", "hole",
	"home", "hood", "hook", "hope", "horn", "hose", "host", "hour",
	"huge", "hull", "hump", "hung", "hunt", "hurt", "husk", "hymn",
	"iced", "idea", "idle", "inch", "info", "into", "iron", "isle",
	"item", "jack", "jade", "jail", "jazz", "jean", "jerk", "jest",
	"jobs", "join", "joke", "jolt", "jump", "june", "jury", "just",
	"keen", "keep", "kept", "kick", "kids", "kill", "kind", "king",
	"kiss", "kite", "knap", "knee", "knew", "knit", "knob", "knot",
	"know", "lace", "lack", "laid", "lake", "lamb", "lame", "lamp",
	"land", "lane", "lank", "lard", "lark", "last", "late", "lawn",
	"lazy", "lead", "leaf", "lean", "leap", "left", "lend", "lens",
	"lent", "less", "lick", "lied", "life", "lift", "like", "limb",
	"lime", "limp", "line", "link", "lion", "list", "live", "load",
	"loan", "lock", "loft", "logo", "lone", "long", "look", "loop",
	"lord", "lore", "lose", "loss", "lost", "loud", "love", "luck",
	"lump", "lung", "lure", "lurk", "lush", "made", "mail", "main",
	"make", "male", "malt", "mane", "many", "maps", "mare", "mark",
	"mars", "mart", "mask", "mass", "mate", "maze", "mead", "meal",
	"mean", "meat", "meet", "meld", "melt", "memo", "mend", "menu",
	"mere", "mesh", "mess", "mild", "mile", "milk", "mill", "mime",
	"mind", "mine", "mint", "miss", "mist", "moat", "mock", "mode",
	"mold", "mole", "mood", "moon", "more", "moss", "most", "moth",
	"move", "much", "mule", "muse", "mush", "must", "mute", "myth",
	"nail", "name", "navy", "near", "neat", "neck", "need", "nest",
	"news", "next", "nice", "nine", "node", "none", "norm", "nose",
	"note", "noun", "nova", "null", "numb", "oath", "obey", "odds",
	"omen", "omit", "once", "only", "onto", "opal", "open", "opts",
	"oral", "orca", "oven", "over", "owed", "owls", "oxen", "pace",
	"pack", "page", "paid", "pail", "pain", "pair", "pale", "palm",
	"pane", "park", "part", "pass", "past", "path", "pave", "pawn",
	"peak", "pear", "peel", "peer", "pelt", "perk", "pest", "pick",
	"pier", "pike", "pile", "pine", "pink", "pipe", "plan", "play",
	"plea", "plot", "plow", "plug", "plum", "plus", "poem", "poet",
	"pole", "poll", "polo", "pond", "pony", "pool", "poor", "pope",
	"pore", "pork", "port", "pose", "post", "pour", "pray", "prey",
	"prop", "prow", "pull", "pulp", "pump", "punk", "pure", "push",
	"quit", "quiz", "race", "rack", "raft", "rage", "raid", "rail",
	"rain", "rake", "ramp", "rang", "rank", "rare", "rash", "rate",
	"rave", "rays", "read", "real", "reap", "rear", "reed", "reef",
	"reel", "rely", "rent", "rest", "rice", "rich", "ride", "rift",
	"ring", "rise", "risk", "road", "roam", "roar", "robe", "rock",
	"rode", "role", "roll", "roof", "room", "root", "rope", "rose",
	"rude", "ruin", "rule", "rung", "rush", "rust", "safe", "sage",
	"said", "sail", "sake", "sale", "salt", "same", "sand", "sane",
	"sang", "sank", "save", "scan", "seal", "seam", "seat", "seed",
	"seek", "seen", "self", "sell", "send", "sent", "sept", "shed",
	"ship", "shop", "shot", "show", "shut", "sick", "side", "sigh",
	"sign", "silk", "silo", "sing", "sink", "site", "size", "skip",
	"slab", "slam", "slap", "sled", "slew", "slim", "slip", "slot",
	"slow", "slug", "snap", "snow", "soak", "soap", "soar", "sock",
	"soda", "soft", "soil", "sold", "sole", "some", "song", "soon",
	"sore", "sort", "soul", "sour", "span", "spar", "spec", "sped",
	"spin", "spit", "spot", "spur", "star", "stay", "stem", "step",
	"stew", "stir", "stop", "stub", "such", "suit", "sulk", "sung",
	"sunk", "sure", "surf", "swan", "swap", "swim", "tabs", "tack",
	"tail", "take", "tale", "talk", "tall", "tame", "tang", "tank",
	"tape", "taps", "tarn", "task", "team", "tear", "tell", "temp",
	"tend", "tent", "term", "test", "text", "that", "them", "then",
	"they", "thin", "this", "tick", "tide", "tidy", "tied", "tier",
	"tile", "till", "tilt", "time", "tint", "tiny", "tire", "toad",
	"toil", "told", "toll", "tomb", "tone", "took", "tool", "tops",
	"tore", "torn", "toss", "tour", "town", "trap", "tray", "tree",
	"trim", "trio", "trip", "trot", "true", "tube", "tuck", "tuft",
	"tune", "turn", "turf", "twig", "twin", "type", "unit", "upon",
	"urge", "used", "user", "vale", "vane", "vary", "vast", "veil",
	"vein", "vent", "verb", "very", "vest", "veto", "vice", "view",
	"vine", "void", "volt", "vote", "wade", "wage", "wait", "wake",
	"walk", "wall", "wand", "want", "ward", "warm", "warn", "warp",
	"wary", "wash", "wave", "wavy", "waxy", "ways", "weak", "wear",
	"weed", "week", "weep", "weld", "well", "went", "were", "west",
	"what", "when", "whom", "wick", "wide", "wife", "wild", "will",
	"wilt", "wily", "wind", "wine", "wing", "wink", "wipe", "wire",
	"wise", "wish", "with", "woke", "wolf", "wood", "wool", "word",
	"wore", "work", "worm", "worn", "wrap", "wren", "yard", "yarn",
	"year", "yell", "your", "zeal", "zero", "zinc", "zone", "zoom",
}

// GeneratePassphrase returns n random words joined by hyphens.
func GeneratePassphrase(n int) (string, error) {
	if n < 1 {
		n = 4
	}
	words := make([]string, n)
	max := big.NewInt(int64(len(wordList)))
	for i := range words {
		idx, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		words[i] = wordList[idx.Int64()]
	}
	return strings.Join(words, "-"), nil
}
