CREATE TABLE IF NOT EXISTS TextData (
    id TEXT PRIMARY KEY,
    user_id INTEGER,
    data TEXT NOT NULL,
    meta_info TEXT,   
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(user_id) REFERENCES Users(id)
);