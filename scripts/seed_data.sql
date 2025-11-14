-- Sample Data Seeding Script
-- Use this to create test data in your database

-- Insert sample users
INSERT INTO users (email, username, password_hash, first_name, last_name, phone) VALUES
('john.doe@example.com', 'johndoe', '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy', 'John', 'Doe', '+1-555-0100'),
('jane.smith@example.com', 'janesmith', '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy', 'Jane', 'Smith', '+1-555-0101'),
('bob.wilson@example.com', 'bobwilson', '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy', 'Bob', 'Wilson', '+1-555-0102'),
('alice.brown@example.com', 'alicebrown', '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy', 'Alice', 'Brown', '+1-555-0103'),
('charlie.davis@example.com', 'charliedavis', '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy', 'Charlie', 'Davis', '+1-555-0104')
ON CONFLICT (email) DO NOTHING;

-- Insert sample products
INSERT INTO products (name, description, price, stock_quantity, category) VALUES
('Laptop Pro 15"', 'High-performance laptop with 16GB RAM', 1299.99, 50, 'Electronics'),
('Wireless Mouse', 'Ergonomic wireless mouse with USB receiver', 29.99, 200, 'Electronics'),
('Office Chair', 'Comfortable ergonomic office chair', 249.99, 30, 'Furniture'),
('Desk Lamp', 'LED desk lamp with adjustable brightness', 39.99, 100, 'Furniture'),
('USB-C Cable', 'High-speed USB-C charging cable', 15.99, 500, 'Accessories'),
('Laptop Backpack', 'Water-resistant laptop backpack', 59.99, 75, 'Accessories'),
('Mechanical Keyboard', 'RGB mechanical gaming keyboard', 129.99, 60, 'Electronics'),
('Monitor 27"', '4K UHD monitor with HDR support', 399.99, 40, 'Electronics'),
('Webcam HD', '1080p HD webcam with microphone', 79.99, 120, 'Electronics'),
('Notebook Set', 'Premium notebook set (3 pack)', 19.99, 300, 'Stationery')
ON CONFLICT DO NOTHING;

-- Insert sample orders
INSERT INTO orders (user_id, total_amount, status, shipping_address) 
SELECT 
    u.id,
    CASE 
        WHEN u.id = 1 THEN 1329.98
        WHEN u.id = 2 THEN 449.98
        WHEN u.id = 3 THEN 159.97
        ELSE 100.00
    END,
    CASE 
        WHEN u.id % 2 = 0 THEN 'completed'
        ELSE 'pending'
    END,
    '123 Main St, Anytown, USA 12345'
FROM users u
LIMIT 3
ON CONFLICT DO NOTHING;

-- Insert sample order items
INSERT INTO order_items (order_id, product_id, quantity, unit_price)
SELECT 
    o.id,
    p.id,
    FLOOR(RANDOM() * 3 + 1)::INTEGER,
    p.price
FROM orders o
CROSS JOIN products p
WHERE p.id <= 5
LIMIT 15
ON CONFLICT DO NOTHING;

-- Display summary
SELECT 
    'Users' AS table_name, 
    COUNT(*) AS row_count 
FROM users
UNION ALL
SELECT 'Products', COUNT(*) FROM products
UNION ALL
SELECT 'Orders', COUNT(*) FROM orders
UNION ALL
SELECT 'Order Items', COUNT(*) FROM order_items
ORDER BY table_name;
