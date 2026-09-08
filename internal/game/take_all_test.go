package game

import (
	"context"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
)

func TestTakeAllCapacityQuestAndConcurrentPickup(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	world := NewWorldService(db)
	user, room, normal, quest := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	_, err := db.Exec(`INSERT INTO users(id,username,email,password_hash) VALUES($1,$2,$3,'test')`, user, user.String(), user.String()+"@example.test")
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO rooms(id,name,description) VALUES($1,'Bulk test','Test room')`, room)
	require.NoError(t, err)
	t.Cleanup(func() {
		db.Exec(`DELETE FROM users WHERE id=$1`, user)
		db.Exec(`DELETE FROM room_items WHERE room_id=$1`, room)
		db.Exec(`DELETE FROM rooms WHERE id=$1`, room)
		db.Exec(`DELETE FROM items WHERE id IN ($1,$2)`, normal, quest)
	})
	_, err = db.Exec(`INSERT INTO items(id,name,description,item_type,properties) VALUES($1,'A quest','A test quest item','quest','{"quest":true}'),($2,'Z pebble','A test pebble','trinket','{}')`, quest, normal)
	require.NoError(t, err)
	chars := []uuid.UUID{uuid.New(), uuid.New()}
	for i, c := range chars {
		_, err = db.Exec(`INSERT INTO characters(id,user_id,name,current_room_id) VALUES($1,$2,$3,$4)`, c, user, "Collector "+string(rune('A'+i)), room)
		require.NoError(t, err)
	}
	_, err = db.Exec(`INSERT INTO room_items(room_id,item_id,quantity) VALUES($1,$2,3),($1,$3,100)`, room, quest, normal)
	require.NoError(t, err)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, c := range chars {
		wg.Add(1)
		go func(c uuid.UUID) { defer wg.Done(); _, e := world.TakeAll(ctx, c, room); errs <- e }(c)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		require.NoError(t, e)
	}
	for _, c := range chars {
		inv := mustInventory(t, world, c)
		sum := 0
		for _, v := range inv {
			sum += v.Quantity
			if v.Item.ID == quest {
				require.Equal(t, 1, v.Quantity)
			}
		}
		require.Equal(t, 50, sum)
	}
	ground, err := world.ListRoomItems(ctx, room)
	require.NoError(t, err)
	sum := 0
	for _, g := range ground {
		sum += g.Quantity
	}
	require.Equal(t, 3, sum)
	taken, err := world.TakeAll(ctx, chars[0], room)
	require.NoError(t, err)
	require.Empty(t, taken)
	_, err = db.Exec(`DELETE FROM inventory WHERE character_id=$1 AND item_id=$2`, chars[0], normal)
	require.NoError(t, err)
	taken, err = world.TakeAll(ctx, chars[0], room)
	require.NoError(t, err)
	require.Len(t, taken, 1)
	require.Equal(t, normal, taken[0].ItemID)
	require.Equal(t, 2, taken[0].Quantity)
	taken, err = world.TakeAll(ctx, chars[0], uuid.New())
	require.Error(t, err)
	require.Empty(t, taken)
}
