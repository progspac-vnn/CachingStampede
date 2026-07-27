-- Sample product catalog for local development and load testing.
-- Safe to re-run: existing rows are left untouched via ON CONFLICT.
INSERT INTO products (name, description, price, stock) VALUES
    ('Wireless Mouse', 'Ergonomic wireless mouse with USB receiver.', 19.99, 150),
    ('Mechanical Keyboard', 'RGB backlit mechanical keyboard, blue switches.', 79.99, 80),
    ('27-inch Monitor', '27" QHD IPS monitor, 144Hz refresh rate.', 249.99, 40),
    ('USB-C Hub', '7-in-1 USB-C hub with HDMI and SD card reader.', 34.50, 200),
    ('Noise Cancelling Headphones', 'Over-ear ANC headphones, 30h battery life.', 129.00, 60)
ON CONFLICT (name) DO NOTHING;
