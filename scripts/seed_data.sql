-- Seed data for testing database migration and anonymization
-- Generates 1000 records for each table

-- Insert 1000 users
WITH inserted_users AS (
    INSERT INTO users (email, username, password_hash, first_name, last_name, phone, test_change, created_at, updated_at)
    SELECT
        'user' || i || '@example.com',
        'user' || i,
        '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy',
        'FirstName' || i,
        'LastName' || i,
        '+1' || LPAD(i::text, 9, '0'),
        CASE WHEN i % 3 = 0 THEN 'test_value_' || i ELSE NULL END,
        NOW() - (i || ' days')::INTERVAL,
        NOW() - (i || ' days')::INTERVAL
    FROM generate_series(1, 1000) AS i
    RETURNING id
)
SELECT COUNT(*) FROM inserted_users;

-- Insert 1000 products
WITH inserted_products AS (
    INSERT INTO products (name, description, price, stock_quantity, category, created_at, updated_at)
    SELECT
        'Product ' || i,
        'Description for product ' || i || '. This is a high-quality item designed for your needs.',
        ROUND((RANDOM() * 1000 + 10)::numeric, 2),
        (RANDOM() * 500)::int,
        CASE 
            WHEN i % 5 = 0 THEN 'Electronics'
            WHEN i % 5 = 1 THEN 'Furniture'
            WHEN i % 5 = 2 THEN 'Clothing'
            WHEN i % 5 = 3 THEN 'Books'
            ELSE 'Home & Garden'
        END,
        NOW() - (i || ' days')::INTERVAL,
        NOW() - (i || ' days')::INTERVAL
    FROM generate_series(1, 1000) AS i
    RETURNING id
)
SELECT COUNT(*) FROM inserted_products;

-- Insert 1000 orders
WITH user_ids AS (
    SELECT id, ROW_NUMBER() OVER (ORDER BY id) as rn
    FROM users
    LIMIT 1000
),
inserted_orders AS (
    INSERT INTO orders (user_id, total_amount, status, shipping_address, created_at, updated_at)
    SELECT
        u.id,
        ROUND((RANDOM() * 2000 + 50)::numeric, 2),
        CASE 
            WHEN i % 4 = 0 THEN 'completed'
            WHEN i % 4 = 1 THEN 'shipped'
            WHEN i % 4 = 2 THEN 'pending'
            ELSE 'processing'
        END,
        i || ' Main Street, City ' || ((i - 1) % 100 + 1) || ', ST ' || LPAD(((i - 1) % 99999 + 10000)::text, 5, '0'),
        NOW() - (i || ' days')::INTERVAL,
        NOW() - (i || ' days')::INTERVAL
    FROM generate_series(1, 1000) AS i
    JOIN user_ids u ON u.rn = ((i - 1) % 1000) + 1
    RETURNING id
)
SELECT COUNT(*) FROM inserted_orders;

-- Insert 1000 order items
WITH order_ids AS (
    SELECT id, ROW_NUMBER() OVER (ORDER BY id) as rn
    FROM orders
    LIMIT 1000
),
product_ids AS (
    SELECT id, ROW_NUMBER() OVER (ORDER BY id) as rn
    FROM products
    LIMIT 1000
)
INSERT INTO order_items (order_id, product_id, quantity, unit_price, created_at)
SELECT
    o.id,
    p.id,
    (RANDOM() * 10 + 1)::int,
    ROUND((RANDOM() * 500 + 10)::numeric, 2),
    NOW() - (i || ' days')::INTERVAL
FROM generate_series(1, 1000) AS i
JOIN order_ids o ON o.rn = ((i - 1) % 1000) + 1
JOIN product_ids p ON p.rn = ((i * 7 - 1) % 1000) + 1;
