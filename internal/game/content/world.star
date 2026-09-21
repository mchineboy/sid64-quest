def build():
    # The first room is the world root. link() always creates a return exit.
    room("square", "Town Square", "A fountain murmurs over cobblestones. The gate is north, the market east, the inn south, and the old town west. Type trails for an explorer's guide.", "safe", "guide")
    room("gate", "North Gate", "Weathered gates frame the docks to the north. East, an old road follows the river toward farms and distant hills.", "safe")
    room("market", "Market Lane", "Canvas awnings snap overhead. Beyond the stalls to the east lies a garden of medicinal herbs.", "shop")
    room("inn", "The Prancing Pony Inn", "A low fire and a room full of rumors welcome travelers. Rest here before following the southern road.", "inn")
    room("docks", "Moonlit Docks", "Black water taps against the pilings. The harbor ledger lies somewhere nearby. A seawall runs east toward a lonely lighthouse.")
    link("square", "north", "gate")
    link("square", "east", "market")
    link("square", "south", "inn")
    link("gate", "north", "docks")

    # Overworld: three trails with a return loop through the countryside.
    for key, name, description, kind in [
        ("old_town", "Old Town", "Narrow houses lean over a lane west of the square. Beyond them a cemetery climbs toward the hills.", "safe"),
        ("cemetery", "Lantern Cemetery", "Blue lanterns mark old graves. A stair descends into the Bellkeeper Crypt; a carved sign promises a trial of memory, not strength.", "normal"),
        ("river_road", "River Road", "Willows trail their fingers in the river. North lies a stone bridge; east, a mill turns beside the water.", "normal"),
        ("bridge", "Willow Bridge", "A broad bridge crosses the silver river. Farm tracks continue north, while the river road leads home to the south.", "normal"),
        ("orchard", "Windfall Orchard", "Apples scent the air. The trees thin northward into whispering woodland; a distant ridge rises above the canopy.", "normal"),
        ("mill", "Mosswheel Mill", "A waterwheel creaks through green water. Flour-dusted footprints lead east toward a woodland lodge.", "normal"),
        ("lodge", "Forester's Lodge", "A kettle steams above a hearth. Travelers may rest here. A trail north reaches a spring, and west lies the mill.", "inn"),
        ("forest", "Whispering Wood", "Leaves hiss like distant voices. A ridge rises north of the trail; to the east, a spring glints between the roots.", "normal"),
        ("spring", "Starlit Spring", "Clear water reflects the sky even beneath the canopy. An eastern trail climbs to a ruined watchtower.", "normal"),
        ("tower", "Broken Watchtower", "From the broken battlements you can trace the river to town and the ridge to an abandoned silver mine.", "normal"),
        ("ridge", "Heather Ridge", "Purple heather bows before a cold wind. East, a mine entrance yawns beneath a rusted bell.", "normal"),
        ("mine_entrance", "Silvervein Mine Entrance", "An abandoned lift descends into Silvervein Mine. A notice reads: Restore the pumps, recover the survey, leave the workings intact.", "normal"),
        ("garden", "Apothecary Garden", "Mint and lavender grow in careful rows. The market lies west; bees hum over the flowerbeds.", "safe"),
        ("common", "South Common", "Kites wheel over open grass. The inn is north, and a coastal path runs south.", "safe"),
        ("coast", "Saltwind Path", "Sea thrift grows between pale stones. Eastward, the path meets the lighthouse keeper's cottage.", "normal"),
        ("seawall", "Harbor Seawall", "Spray leaps against the wall. The docks lie west and the lighthouse rises east above a cave-pierced headland.", "normal"),
        ("lighthouse", "Lighthouse Headland", "A beacon sweeps the sea. Steps descend into the Tideglass Grotto. South stands the keeper's welcoming cottage.", "normal"),
        ("cottage", "Keeper's Cottage", "The keeper has left a fire and spare blankets for travelers. Rest here before exploring the grotto beneath the lighthouse.", "inn"),
    ]:
        room(key, name, description, kind)
    link("square", "west", "old_town")
    link("old_town", "west", "cemetery")
    link("gate", "east", "river_road")
    link("river_road", "north", "bridge")
    link("bridge", "north", "orchard")
    link("river_road", "east", "mill")
    link("mill", "east", "lodge")
    link("lodge", "north", "spring")
    link("orchard", "north", "forest")
    link("forest", "east", "spring")
    link("spring", "east", "tower")
    link("forest", "north", "ridge")
    link("ridge", "east", "mine_entrance")
    link("market", "east", "garden")
    link("inn", "south", "common")
    link("common", "south", "coast")
    link("coast", "east", "cottage")
    link("docks", "east", "seawall")
    link("seawall", "east", "lighthouse")
    link("lighthouse", "south", "cottage")

    # Each dungeon has a safe return stair, side chambers with clues, and a finale.
    for key, name, description in [
        ("crypt_stair", "Crypt Lantern Stair", "Blue lanterns light dry steps. Up returns to the cemetery; north leads into the Bellkeeper Crypt. No door closes behind you."),
        ("crypt_hall", "Processional Hall", "Stone mourners face a northern reliquary. An eastern arch leads to the choir, and a western arch to a scribe's resting place."),
        ("crypt_choir", "Silent Choir", "A mural depicts a bronze bell sending ripples through the air. Beneath it: I have a tongue, but never speak."),
        ("crypt_scribe", "Scribe's Rest", "A slate reads: I call the town from sleep, yet never wake myself. The answer waits in the reliquary to the north of the hall."),
        ("crypt_reliquary", "Bellkeeper Reliquary", "A sealed offering bowl bears a riddle: With a tongue of bronze I call the town, yet have no breath. Answer with: answer <word>."),
        ("crypt_ossuary", "Garden of Bones", "Tiny white flowers grow among ancient stones. A plaque asks visitors to leave the sleepers undisturbed. The reliquary lies west."),
    ]:
        room(key, name, description, "dungeon", "crypt")
    link("cemetery", "down", "crypt_stair")
    link("crypt_stair", "north", "crypt_hall")
    link("crypt_hall", "east", "crypt_choir")
    link("crypt_hall", "west", "crypt_scribe")
    link("crypt_hall", "north", "crypt_reliquary")
    link("crypt_reliquary", "east", "crypt_ossuary")
    link("crypt_choir", "north", "crypt_ossuary")

    for key, name, description in [
        ("mine_lift", "Silvervein Lift Landing", "The lift rests on solid stone. Up returns to daylight. North, rail tracks lead to a junction."),
        ("mine_junction", "Ore Cart Junction", "Rails branch east to a survey office, west to a flooded gallery, and north to the pumping chamber."),
        ("mine_office", "Survey Office", "A pinned diagram reads: First open the INTAKE. Then engage the WHEEL. Only then release the SLUICE. The pumps lie north of the junction."),
        ("mine_gallery", "Flooded Gallery", "Still water covers the lower workings. A warning reads: A wrong pump setting resets the sequence. No need to enter the water."),
        ("mine_pumps", "Silvervein Pump Chamber", "Three brass controls are labeled intake, wheel, and sluice. Operate them with crank <label>. A locked survey case is connected to the pump."),
        ("mine_seam", "Crystal Seam", "Unmined crystals scatter lamplight like stars. An old miner's verse praises the beauty of what is left in the earth."),
    ]:
        room(key, name, description, "dungeon", "mine")
    link("mine_entrance", "down", "mine_lift")
    link("mine_lift", "north", "mine_junction")
    link("mine_junction", "east", "mine_office")
    link("mine_junction", "west", "mine_gallery")
    link("mine_junction", "north", "mine_pumps")
    link("mine_pumps", "east", "mine_seam")
    link("mine_office", "north", "mine_seam")

    for key, name, description in [
        ("grotto_steps", "Tideglass Steps", "Lighthouse steps end above the high-water mark. Up returns to the headland; north leads into luminous caves."),
        ("grotto_pool", "Mirror Pool", "A dry ledge circles a pool of stars. Shells gleam east, a tidal chart hangs west, and the lens chamber lies north."),
        ("grotto_shells", "Shell Archive", "Shell mosaics preserve a keeper's lesson: The MOON draws the water. The TIDE carries the vessel. The BEACON brings it home."),
        ("grotto_chart", "Tidal Chart Room", "A chart marks a safe path above every tide. A note says: Align the lens in the order of the keeper's lesson; a mistake begins the lesson again."),
        ("grotto_lens", "Tideglass Lens Chamber", "A forgotten lens is ringed by symbols of moon, tide, and beacon. Use align <symbol> to restore its light."),
        ("grotto_window", "Sea Window", "Beyond a stone balcony, waves glow with tiny blue lights. The lens chamber lies west; the shell archive is south."),
    ]:
        room(key, name, description, "dungeon", "grotto")
    link("lighthouse", "down", "grotto_steps")
    link("grotto_steps", "north", "grotto_pool")
    link("grotto_pool", "east", "grotto_shells")
    link("grotto_pool", "west", "grotto_chart")
    link("grotto_pool", "north", "grotto_lens")
    link("grotto_lens", "east", "grotto_window")
    link("grotto_shells", "north", "grotto_window")

    # Three outland branches plus five paths that tie the regions together.
    for key, name, description, kind in [
        ("eastvale_road", "Eastvale Road", "Cart ruts leave Willow Bridge and run east between hedgerows. A fen track leaves the verge to the north.", "normal"),
        ("eastvale_green", "Eastvale Green", "Geese patrol a mown common. The grange stands north; a dovecote rise lies east.", "normal"),
        ("eastvale_grange", "Harrowgate Grange", "Barley sacks and clean straw. Travelers may rest here. The green lies south and terraces east.", "inn"),
        ("eastvale_dovecote", "Dovecote Rise", "White birds wheel around a stone tower. Fallow terraces climb north.", "normal"),
        ("eastvale_fallow", "Fallow Terrace", "Stubble steps down toward the river. A grass-covered barrow waits east.", "normal"),
        ("eastvale_barrow", "Vaultward Barrow", "A collapsed mound exposes tithe-stones and a stair down. A notice: count honestly, take only your share.", "normal"),
        ("stormbreak_dunes", "Stormbreak Dunes", "Marram grass hisses over shifting sand. The strand lies south; cut steps climb east.", "normal"),
        ("stormbreak_strand", "Wrackline Strand", "Kelp and broken planking mark the last high tide. A spit runs east, a dead beacon stands south.", "normal"),
        ("stormbreak_spit", "Gullbone Spit", "Gulls quarrel over a shingle bar. A wrecker's hut sits south.", "normal"),
        ("stormbreak_hut", "Wrecker's Hut", "Driftwood walls and a banked fire. Travelers may rest here. The hulk lies south.", "inn"),
        ("stormbreak_beacon", "Broken Beacon", "A toppled signal tower rusts in the spray. The hut lies east.", "normal"),
        ("stormbreak_hulk", "Tideworn Hulk", "A gutted ship lies canted on the flats, its hold open below. A warning: rig the capstan in the purser's order.", "normal"),
        ("highland_crown", "Watchtower Crown", "Wind sings through broken merlons. A cairn pass climbs north; open heath runs west.", "normal"),
        ("highland_pass", "Cairn Pass", "Stacked cairns mark the safe line. A tarn glints north; a bothy sits east.", "normal"),
        ("highland_tarn", "Frostmere Tarn", "Black water keeps a rim of ice all summer. A scree shelf lies east.", "normal"),
        ("highland_bothy", "Shepherd's Bothy", "Peat smoke and dry blankets. Travelers may rest here. Scree climbs north.", "inn"),
        ("highland_scree", "Scree Shelf", "Loose stone slides underfoot. A cold mouth breathes north.", "normal"),
        ("highland_mouth", "Frostfall Mouth", "Rime coats a shaft dropping into Frostfall Deep. A warning: wake the forge before the cold finds you.", "normal"),
        ("fen_track", "Osier Fen Track", "Willow whips edge a boggy path. The orchard lies west; Eastvale Road runs south.", "normal"),
        ("heath_cut", "Windcut Heath", "Flattened heather between the low ridge and the high country. The ridge is south, the tower crown east.", "normal"),
        ("gullway", "Gullway Steps", "Cut steps drop from the keeper's clifftop to the sand. The cottage is north, the dunes west.", "normal"),
        ("toll_span", "Old Toll Span", "An empty tollhouse straddles a chasm bridge. The mine entrance lies west; a rimed mouth opens east.", "normal"),
        ("lantern_walk", "Lanternwright Walk", "Oil lamps hang from iron brackets. The garden is north and the common lies west.", "safe"),
    ]:
        room(key, name, description, kind)
    for a, direction, b in [
        ("bridge", "east", "eastvale_road"),
        ("eastvale_road", "east", "eastvale_green"),
        ("eastvale_green", "north", "eastvale_grange"),
        ("eastvale_green", "east", "eastvale_dovecote"),
        ("eastvale_grange", "east", "eastvale_fallow"),
        ("eastvale_dovecote", "north", "eastvale_fallow"),
        ("eastvale_fallow", "east", "eastvale_barrow"),
        ("coast", "south", "stormbreak_dunes"),
        ("stormbreak_dunes", "south", "stormbreak_strand"),
        ("stormbreak_strand", "east", "stormbreak_spit"),
        ("stormbreak_strand", "south", "stormbreak_beacon"),
        ("stormbreak_spit", "south", "stormbreak_hut"),
        ("stormbreak_beacon", "east", "stormbreak_hut"),
        ("stormbreak_hut", "south", "stormbreak_hulk"),
        ("tower", "up", "highland_crown"),
        ("highland_crown", "north", "highland_pass"),
        ("highland_pass", "north", "highland_tarn"),
        ("highland_pass", "east", "highland_bothy"),
        ("highland_tarn", "east", "highland_scree"),
        ("highland_bothy", "north", "highland_scree"),
        ("highland_scree", "north", "highland_mouth"),
        ("orchard", "east", "fen_track"),
        ("fen_track", "south", "eastvale_road"),
        ("ridge", "north", "heath_cut"),
        ("heath_cut", "east", "highland_crown"),
        ("cottage", "south", "gullway"),
        ("gullway", "west", "stormbreak_dunes"),
        ("mine_entrance", "east", "toll_span"),
        ("toll_span", "east", "highland_mouth"),
        ("garden", "south", "lantern_walk"),
        ("lantern_walk", "west", "common"),
    ]:
        link(a, direction, b)

    for key, name, description in [
        ("vault_stair", "Tithe Stair", "Dry steps descend beneath the barrow. Up returns to the fields; north opens the counting hall."),
        ("vault_hall", "Counting Hall", "Stone tables hold empty measures. A ledger alcove lies east, a sunken granary west, the strongroom north."),
        ("vault_ledger", "Ledger Alcove", "A tally board reads: The GRAIN is measured, then the COIN is weighed, and only then the SEAL is pressed."),
        ("vault_granary", "Sunken Granary", "Split sacks spill blackened barley. A steward's note warns that a wrong plate returns the tally to nothing."),
        ("vault_strongroom", "Tithe Strongroom", "Three worn plates are cut with grain, coin, and seal. Work them with press <plate>. A sealed tithe box waits alongside."),
        ("vault_cellar", "Warden's Cellar", "Cold shelves and a warden's cot. The strongroom lies west; the ledger alcove is south."),
    ]:
        room(key, name, description, "dungeon", "vault")

    for key, name, description in [
        ("wreck_steps", "Wrackstair", "Salt-eaten steps leave the canted deck. Up returns to the shore; north enters the flooded hold."),
        ("wreck_hold", "Flooded Hold", "Shallow water laps over ballast stone. A galley lies east, a bosun's locker west, the capstan deck north."),
        ("wreck_galley", "Drowned Galley", "Scratched into the mess table: Set the ANCHOR, raise the SPAR, and last of all the SAIL."),
        ("wreck_locker", "Bosun's Locker", "Coiled rope and a slate: rigging worked out of order must be begun again from the anchor."),
        ("wreck_capstan", "Capstan Deck", "Three pins stand at the capstan, marked anchor, spar, and sail. Work them with rig <pin>. A purser's chest is lashed nearby."),
        ("wreck_prow", "Shattered Prow", "The bow gapes open on the sea floor. The capstan deck lies west; the galley is south."),
    ]:
        room(key, name, description, "dungeon", "wreck")

    for key, name, description in [
        ("frost_steps", "Rimeward Steps", "Ice-cut steps drop from the mouth. Up returns to the shelf; north opens a glacier cavern."),
        ("frost_cavern", "Glacier Cavern", "Blue ice arches overhead. A verse wall stands east, a hoarfrost shelf west, and the frozen forge north."),
        ("frost_verse", "Ice Verse Wall", "Carved letters hold the frost: The TINDER takes the spark, the BELLOWS wake the coals, the FLUE draws the smoke."),
        ("frost_shelf", "Hoarfrost Shelf", "Frozen tools lie where they fell. A smith's note: stoke them out of order and the fire dies to nothing."),
        ("frost_forge", "Frozen Forge", "A dead hearth is ringed by levers for tinder, bellows, and flue. Work them with stoke <lever>. A smith's strongbox is frozen shut."),
        ("frost_gallery", "Blue Gallery", "Ice columns ring like struck glass. The forge lies west; the verse wall is south."),
    ]:
        room(key, name, description, "dungeon", "frost")

    # A quiet room beside the crypt stair for the newly dead; no combat here.
    room("hall_returning", "Hall of Returning", "Wardens lay the fallen on benches facing a slow bell. A coin bowl stands by the door, and nothing hostile crosses the threshold. The lantern stair lies west.", "safe", "returning")

    for a, direction, b in [
        ("eastvale_barrow", "down", "vault_stair"),
        ("vault_stair", "north", "vault_hall"),
        ("vault_hall", "east", "vault_ledger"),
        ("vault_hall", "west", "vault_granary"),
        ("vault_hall", "north", "vault_strongroom"),
        ("vault_strongroom", "east", "vault_cellar"),
        ("vault_ledger", "north", "vault_cellar"),
        ("stormbreak_hulk", "down", "wreck_steps"),
        ("wreck_steps", "north", "wreck_hold"),
        ("wreck_hold", "east", "wreck_galley"),
        ("wreck_hold", "west", "wreck_locker"),
        ("wreck_hold", "north", "wreck_capstan"),
        ("wreck_capstan", "east", "wreck_prow"),
        ("wreck_galley", "north", "wreck_prow"),
        ("highland_mouth", "down", "frost_steps"),
        ("frost_steps", "north", "frost_cavern"),
        ("frost_cavern", "east", "frost_verse"),
        ("frost_cavern", "west", "frost_shelf"),
        ("frost_cavern", "north", "frost_forge"),
        ("frost_forge", "east", "frost_gallery"),
        ("frost_verse", "north", "frost_gallery"),
        ("crypt_stair", "east", "hall_returning"),
    ]:
        link(a, direction, b)

    monster("crypt_sentinel", "crypt_choir", "Bronze Sentinel", "A hollow bronze guardian raises a chipped ceremonial blade.", health=24, attack=6, defense=1, gold=8, experience=15, loot="Warden Mail", drop=300)
    monster("crypt_wraith", "crypt_ossuary", "Lantern Wraith", "Cold blue light gathers around an ancient keeper's shadow.", health=36, attack=8, defense=2, gold=12, experience=25, loot="Warden Mail", drop=300)
    monster("mine_rat", "mine_gallery", "Tunnel Rat", "A large gray rat guards a nest of stolen copper wire.", health=18, attack=4, gold=5, experience=10, respawn=120, loot="Reinforced Hide", drop=800)
    monster("mine_golem", "mine_seam", "Quartz Golem", "A slow stone construct grinds crystal fists together.", health=45, attack=10, defense=3, gold=18, experience=35, loot="Runed Plate", drop=100)
    monster("grotto_crab", "grotto_shells", "Glassback Crab", "A translucent shell protects a pair of snapping claws.", health=28, attack=7, defense=2, gold=10, experience=20, loot="Reinforced Hide", drop=800)
    monster("grotto_eel", "grotto_window", "Storm Eel", "A ribbon of electric blue coils over the wet stone.", health=38, attack=9, defense=1, gold=15, experience=30, loot="Warden Mail", drop=300)
    monster("vault_thresher", "vault_granary", "Chaff Thresher", "A harvest flail turns by itself above the spilled grain.", health=30, attack=7, defense=2, gold=14, experience=24, respawn=180, loot="Reinforced Hide", drop=800)
    monster("vault_warden", "vault_cellar", "Tithe Warden", "A dusty figure still counts coins that are no longer there.", health=42, attack=9, defense=3, gold=20, experience=34, loot="Runed Plate", drop=100)
    monster("wreck_lamprey", "wreck_galley", "Hold Lamprey", "A pale coil unwinds from the bilge with a ring of teeth.", health=32, attack=8, defense=1, gold=12, experience=22, respawn=150, loot="Reinforced Hide", drop=800)
    monster("wreck_figurehead", "wreck_prow", "Salt-Bitten Figurehead", "A carved woman tears free of the bow and turns her blind eyes on you.", health=46, attack=10, defense=3, gold=22, experience=38, loot="Runed Plate", drop=100)
    monster("frost_hare", "frost_shelf", "Rimeclaw Hare", "A white hare the size of a hound bares frost-glazed claws.", health=26, attack=6, defense=1, gold=9, experience=18, respawn=120, loot="Warden Mail", drop=300)
    monster("frost_smith", "frost_gallery", "Hollow Smith", "An empty apron and gauntlets swing a hammer of clear ice.", health=50, attack=11, defense=4, gold=25, experience=42, loot="Crownward Aegis", drop=25)

build()
