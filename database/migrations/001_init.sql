-- =============================================================
-- Enterprise Knowledge Portal - Database Schema
-- =============================================================

-- Enable extensions
CREATE EXTENSION IF NOT EXISTS "pgcrypto";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";  -- for full-text search

-- =============================================================
-- Users & Authentication
-- =============================================================

CREATE TABLE IF NOT EXISTS users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email         VARCHAR(255) UNIQUE NOT NULL,
    name          VARCHAR(255) NOT NULL,
    role          VARCHAR(50) DEFAULT 'user',       -- user, admin, analyst
    okta_id       VARCHAR(255),
    avatar_url    TEXT,
    department    VARCHAR(100),
    is_active     BOOLEAN DEFAULT true,
    last_login_at TIMESTAMP,
    created_at    TIMESTAMP DEFAULT NOW(),
    updated_at    TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_users_email  ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_okta   ON users(okta_id);

-- =============================================================
-- Enterprise Data - Core Tables
-- =============================================================

CREATE TABLE IF NOT EXISTS enterprise_departments (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name                VARCHAR(100) NOT NULL,
    code                VARCHAR(20) UNIQUE NOT NULL,
    head_of_department  VARCHAR(100),
    budget              NUMERIC(15,2),
    employee_count      INTEGER DEFAULT 0,
    location            VARCHAR(100),
    created_at          TIMESTAMP DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS enterprise_employees (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    employee_id   VARCHAR(20) UNIQUE NOT NULL,
    first_name    VARCHAR(100) NOT NULL,
    last_name     VARCHAR(100) NOT NULL,
    email         VARCHAR(255) UNIQUE NOT NULL,
    department_id UUID REFERENCES enterprise_departments(id),
    job_title     VARCHAR(100),
    salary        NUMERIC(12,2),
    hire_date     DATE,
    status        VARCHAR(20) DEFAULT 'active',     -- active, inactive, on_leave
    location      VARCHAR(100),
    manager_id    UUID REFERENCES enterprise_employees(id),
    phone         VARCHAR(20),
    created_at    TIMESTAMP DEFAULT NOW(),
    updated_at    TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_employees_dept    ON enterprise_employees(department_id);
CREATE INDEX IF NOT EXISTS idx_employees_manager ON enterprise_employees(manager_id);
CREATE INDEX IF NOT EXISTS idx_employees_status  ON enterprise_employees(status);

CREATE TABLE IF NOT EXISTS enterprise_products (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    sku            VARCHAR(50) UNIQUE NOT NULL,
    name           VARCHAR(255) NOT NULL,
    category       VARCHAR(100),
    description    TEXT,
    unit_price     NUMERIC(12,2),
    cost_price     NUMERIC(12,2),
    stock_quantity INTEGER DEFAULT 0,
    reorder_level  INTEGER DEFAULT 10,
    supplier       VARCHAR(150),
    is_active      BOOLEAN DEFAULT true,
    created_at     TIMESTAMP DEFAULT NOW(),
    updated_at     TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_products_category ON enterprise_products(category);
CREATE INDEX IF NOT EXISTS idx_products_sku      ON enterprise_products(sku);

CREATE TABLE IF NOT EXISTS enterprise_sales (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    transaction_id VARCHAR(50) UNIQUE NOT NULL,
    product_id     UUID REFERENCES enterprise_products(id),
    employee_id    UUID REFERENCES enterprise_employees(id),
    quantity       INTEGER NOT NULL,
    unit_price     NUMERIC(12,2) NOT NULL,
    total_amount   NUMERIC(14,2) NOT NULL,
    customer_name  VARCHAR(255),
    customer_email VARCHAR(255),
    region         VARCHAR(100),
    sale_date      TIMESTAMP NOT NULL,
    status         VARCHAR(50) DEFAULT 'completed', -- completed, refunded, pending
    created_at     TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_sales_product  ON enterprise_sales(product_id);
CREATE INDEX IF NOT EXISTS idx_sales_employee ON enterprise_sales(employee_id);
CREATE INDEX IF NOT EXISTS idx_sales_date     ON enterprise_sales(sale_date);
CREATE INDEX IF NOT EXISTS idx_sales_region   ON enterprise_sales(region);

CREATE TABLE IF NOT EXISTS enterprise_inventory (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id    UUID REFERENCES enterprise_products(id),
    warehouse     VARCHAR(100) NOT NULL,
    quantity      INTEGER NOT NULL DEFAULT 0,
    location_code VARCHAR(50),
    last_updated  TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_inventory_product   ON enterprise_inventory(product_id);
CREATE INDEX IF NOT EXISTS idx_inventory_warehouse ON enterprise_inventory(warehouse);

-- =============================================================
-- File Management
-- =============================================================

CREATE TABLE IF NOT EXISTS uploaded_files (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID REFERENCES users(id) ON DELETE SET NULL,
    original_name VARCHAR(255) NOT NULL,
    stored_name   VARCHAR(255) NOT NULL,
    file_type     VARCHAR(20) NOT NULL,   -- csv, pdf, docx, txt
    mime_type     VARCHAR(100),
    file_size     BIGINT,
    storage_path  TEXT,                   -- local path or GCS uri
    status        VARCHAR(50) DEFAULT 'processing',  -- processing, ready, error
    row_count     INTEGER,
    error_message TEXT,
    metadata      JSONB DEFAULT '{}',
    created_at    TIMESTAMP DEFAULT NOW(),
    updated_at    TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_files_user   ON uploaded_files(user_id);
CREATE INDEX IF NOT EXISTS idx_files_status ON uploaded_files(status);
CREATE INDEX IF NOT EXISTS idx_files_type   ON uploaded_files(file_type);

CREATE TABLE IF NOT EXISTS file_chunks (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    file_id     UUID REFERENCES uploaded_files(id) ON DELETE CASCADE,
    chunk_index INTEGER NOT NULL,
    content     TEXT NOT NULL,
    metadata    JSONB DEFAULT '{}',
    created_at  TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_chunks_file ON file_chunks(file_id);

-- Full-text search index on chunks
CREATE INDEX IF NOT EXISTS idx_chunks_fts ON file_chunks USING gin(to_tsvector('english', content));

-- =============================================================
-- AI Query History
-- =============================================================

CREATE TABLE IF NOT EXISTS query_history (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID REFERENCES users(id) ON DELETE SET NULL,
    query_text       TEXT NOT NULL,
    query_type       VARCHAR(50),        -- nl_to_sql, document_qa, general
    generated_sql    TEXT,
    result_summary   TEXT,
    result_count     INTEGER,
    execution_time   INTEGER,            -- milliseconds
    status           VARCHAR(50) DEFAULT 'success',
    error_message    TEXT,
    context_file_ids TEXT[],             -- files used as context
    created_at       TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_qhistory_user ON query_history(user_id);
CREATE INDEX IF NOT EXISTS idx_qhistory_type ON query_history(query_type);

-- =============================================================
-- Analytics Reports
-- =============================================================

CREATE TABLE IF NOT EXISTS analytics_reports (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID REFERENCES users(id) ON DELETE SET NULL,
    title        VARCHAR(255) NOT NULL,
    report_type  VARCHAR(100),           -- sales_summary, inventory, hr_overview, custom
    parameters   JSONB DEFAULT '{}',
    result       JSONB DEFAULT '{}',
    status       VARCHAR(50) DEFAULT 'pending',
    generated_at TIMESTAMP,
    created_at   TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_reports_user ON analytics_reports(user_id);

-- =============================================================
-- Seed default admin user
-- =============================================================

INSERT INTO users (email, name, role, department, is_active)
VALUES ('admin@enterprise.com', 'System Admin', 'admin', 'IT', true)
ON CONFLICT (email) DO NOTHING;
