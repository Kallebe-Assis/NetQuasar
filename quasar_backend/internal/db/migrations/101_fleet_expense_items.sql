-- +goose Up
CREATE TABLE fleet_expense_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    expense_id UUID NOT NULL REFERENCES fleet_expenses(id) ON DELETE CASCADE,
    description TEXT NOT NULL,
    quantity DOUBLE PRECISION NOT NULL CHECK (quantity > 0),
    unit_price DOUBLE PRECISION NOT NULL CHECK (unit_price >= 0),
    total_amount DOUBLE PRECISION NOT NULL CHECK (total_amount >= 0),
    sort_order INT NOT NULL DEFAULT 0
);

CREATE INDEX fleet_expense_items_expense_idx ON fleet_expense_items (expense_id, sort_order);

INSERT INTO fleet_expense_items (expense_id, description, quantity, unit_price, total_amount, sort_order)
SELECT id, description, quantity, unit_price, total_amount, 0
FROM fleet_expenses
WHERE NOT EXISTS (
    SELECT 1 FROM fleet_expense_items i WHERE i.expense_id = fleet_expenses.id
);

-- +goose Down
DROP TABLE IF EXISTS fleet_expense_items;
