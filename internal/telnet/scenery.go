package telnet

import "strings"

// Scenery replies never override an actual portable item with the same name.
func sceneryTakeReply(room, query string) string {
	query = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(query)), "the ")
	if query == "lamppost" || query == "lamp post" {
		return "You can't take the lamppost. The town has a strict bring-your-own-lighting policy."
	}
	replies := map[string]map[string]string{
		"Town Square":           {"fountain": "You can't take the fountain. It refuses to travel without its plumbing.", "cobblestones": "The cobblestones are holding down the town. Someone has to."},
		"North Gate":            {"gate": "You can't take the gate. That would leave the town rather open-minded.", "guard": "The guard declines to become inventory."},
		"Market Lane":           {"awning": "You tug the awning. The shopkeeper raises an eyebrow and your potential repair bill.", "awnings": "The awnings are attached. So are the shopkeepers."},
		"The Prancing Pony Inn": {"bar": "You can't take the bar. The regulars have already called dibs.", "fire": "You reconsider carrying fire in your pockets."},
		"Moonlit Docks":         {"boat": "The boat is tied up in nautical red tape.", "water": "You take a handful of water. It immediately escapes.", "pier": "You can't take the pier. It's supporting the local economy."},
	}
	return replies[room][query]
}
