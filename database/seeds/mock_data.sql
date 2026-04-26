-- =============================================================
-- Enterprise Knowledge Portal - Mock Seed Data
-- Run: make seed  (or psql -U portal_user -d enterprise_portal -f mock_data.sql)
-- =============================================================

-- ─── Departments ────────────────────────────────────────────────────────────

INSERT INTO enterprise_departments (id, name, code, head_of_department, budget, employee_count, location) VALUES
  ('d1000000-0000-0000-0000-000000000001', 'Engineering',        'ENG',  'Sarah Mitchell',   2500000.00, 45, 'San Francisco, CA'),
  ('d1000000-0000-0000-0000-000000000002', 'Sales',              'SLS',  'Marcus Thompson',  1800000.00, 38, 'New York, NY'),
  ('d1000000-0000-0000-0000-000000000003', 'Marketing',          'MKT',  'Priya Sharma',     1200000.00, 22, 'Chicago, IL'),
  ('d1000000-0000-0000-0000-000000000004', 'Human Resources',    'HR',   'James Whitfield',   800000.00, 12, 'Austin, TX'),
  ('d1000000-0000-0000-0000-000000000005', 'Finance',            'FIN',  'Linda Park',       1100000.00, 18, 'Seattle, WA'),
  ('d1000000-0000-0000-0000-000000000006', 'Operations',         'OPS',  'David Rodriguez',  950000.00,  25, 'Denver, CO'),
  ('d1000000-0000-0000-0000-000000000007', 'Customer Success',   'CS',   'Emma Wilson',       750000.00, 20, 'Boston, MA')
ON CONFLICT DO NOTHING;

-- ─── Employees ──────────────────────────────────────────────────────────────

INSERT INTO enterprise_employees (id, employee_id, first_name, last_name, email, department_id, job_title, salary, hire_date, status, location) VALUES
  ('e1000000-0000-0000-0000-000000000001', 'EMP001', 'Sarah',    'Mitchell',   'sarah.mitchell@enterprise.com',   'd1000000-0000-0000-0000-000000000001', 'VP Engineering',        145000, '2018-03-15', 'active', 'San Francisco, CA'),
  ('e1000000-0000-0000-0000-000000000002', 'EMP002', 'Marcus',   'Thompson',   'marcus.thompson@enterprise.com',  'd1000000-0000-0000-0000-000000000002', 'VP Sales',              138000, '2017-07-01', 'active', 'New York, NY'),
  ('e1000000-0000-0000-0000-000000000003', 'EMP003', 'Priya',    'Sharma',     'priya.sharma@enterprise.com',     'd1000000-0000-0000-0000-000000000003', 'VP Marketing',          132000, '2019-01-20', 'active', 'Chicago, IL'),
  ('e1000000-0000-0000-0000-000000000004', 'EMP004', 'James',    'Whitfield',  'james.whitfield@enterprise.com',  'd1000000-0000-0000-0000-000000000004', 'HR Director',           125000, '2016-09-05', 'active', 'Austin, TX'),
  ('e1000000-0000-0000-0000-000000000005', 'EMP005', 'Linda',    'Park',       'linda.park@enterprise.com',       'd1000000-0000-0000-0000-000000000005', 'CFO',                   155000, '2015-04-12', 'active', 'Seattle, WA'),
  ('e1000000-0000-0000-0000-000000000006', 'EMP006', 'Alex',     'Chen',       'alex.chen@enterprise.com',        'd1000000-0000-0000-0000-000000000001', 'Senior Engineer',        115000, '2020-06-01', 'active', 'San Francisco, CA'),
  ('e1000000-0000-0000-0000-000000000007', 'EMP007', 'Jordan',   'Lee',        'jordan.lee@enterprise.com',       'd1000000-0000-0000-0000-000000000001', 'Software Engineer',       95000, '2021-08-15', 'active', 'San Francisco, CA'),
  ('e1000000-0000-0000-0000-000000000008', 'EMP008', 'Taylor',   'Brown',      'taylor.brown@enterprise.com',     'd1000000-0000-0000-0000-000000000002', 'Senior Account Exec',    105000, '2019-03-10', 'active', 'New York, NY'),
  ('e1000000-0000-0000-0000-000000000009', 'EMP009', 'Morgan',   'Davis',      'morgan.davis@enterprise.com',     'd1000000-0000-0000-0000-000000000002', 'Account Executive',       85000, '2022-01-10', 'active', 'New York, NY'),
  ('e1000000-0000-0000-0000-000000000010', 'EMP010', 'Riley',    'Garcia',     'riley.garcia@enterprise.com',     'd1000000-0000-0000-0000-000000000003', 'Marketing Manager',       92000, '2020-11-01', 'active', 'Chicago, IL'),
  ('e1000000-0000-0000-0000-000000000011', 'EMP011', 'Casey',    'Martinez',   'casey.martinez@enterprise.com',   'd1000000-0000-0000-0000-000000000004', 'HR Manager',              78000, '2021-04-15', 'active', 'Austin, TX'),
  ('e1000000-0000-0000-0000-000000000012', 'EMP012', 'Drew',     'Johnson',    'drew.johnson@enterprise.com',     'd1000000-0000-0000-0000-000000000005', 'Financial Analyst',       88000, '2021-07-20', 'active', 'Seattle, WA'),
  ('e1000000-0000-0000-0000-000000000013', 'EMP013', 'Quinn',    'Williams',   'quinn.williams@enterprise.com',   'd1000000-0000-0000-0000-000000000006', 'Operations Manager',      98000, '2020-02-28', 'active', 'Denver, CO'),
  ('e1000000-0000-0000-0000-000000000014', 'EMP014', 'Avery',    'Jones',      'avery.jones@enterprise.com',      'd1000000-0000-0000-0000-000000000007', 'Customer Success Mgr',    82000, '2021-09-01', 'active', 'Boston, MA'),
  ('e1000000-0000-0000-0000-000000000015', 'EMP015', 'Skyler',   'Anderson',   'skyler.anderson@enterprise.com',  'd1000000-0000-0000-0000-000000000001', 'DevOps Engineer',        108000, '2020-05-18', 'active', 'San Francisco, CA')
ON CONFLICT DO NOTHING;

-- ─── Products ────────────────────────────────────────────────────────────────

INSERT INTO enterprise_products (id, sku, name, category, description, unit_price, cost_price, stock_quantity, reorder_level, supplier, is_active) VALUES
  ('p1000000-0000-0000-0000-000000000001', 'SOFT-CRM-001',  'Enterprise CRM Suite',          'Software',        'Full-featured CRM platform for enterprise teams',                  2499.00,  400.00, 500,  50, 'TechCorp Solutions',   true),
  ('p1000000-0000-0000-0000-000000000002', 'SOFT-ERP-001',  'ERP Pro Platform',              'Software',        'End-to-end ERP solution with modules for finance, HR, and ops',    4999.00,  800.00, 250,  25, 'OmniSoft Inc',         true),
  ('p1000000-0000-0000-0000-000000000003', 'SOFT-BI-001',   'Business Intelligence Tool',   'Software',        'Advanced BI dashboard and analytics platform',                     1899.00,  300.00, 750,  75, 'DataWise Ltd',         true),
  ('p1000000-0000-0000-0000-000000000004', 'SOFT-COLLAB-01','TeamCollaborate Pro',           'Software',        'Real-time collaboration and project management tool',               599.00,   80.00, 1200, 100,'CollabTech',           true),
  ('p1000000-0000-0000-0000-000000000005', 'SOFT-SEC-001',  'Security Suite Enterprise',    'Software',        'Comprehensive cybersecurity platform with AI threat detection',     3299.00,  550.00, 300,  30, 'SecureNet Corp',       true),
  ('p1000000-0000-0000-0000-000000000006', 'HW-LAPTOP-001', 'UltraBook Pro 15',             'Hardware',        '15-inch high-performance laptop for enterprise use',                1499.00,  950.00, 180,  20, 'TechGear Manufacturing',true),
  ('p1000000-0000-0000-0000-000000000007', 'HW-SERVER-001', 'RackServer X4000',             'Hardware',        '2U rack-mounted server with 64-core CPU and 512GB RAM',            8999.00, 5500.00,  45,   5, 'ServerPro Solutions',   true),
  ('p1000000-0000-0000-0000-000000000008', 'HW-SWITCH-001', 'NetSwitch 48-Port',            'Hardware',        '48-port gigabit managed switch',                                    799.00,  450.00, 120,  15, 'NetEquip Ltd',         true),
  ('p1000000-0000-0000-0000-000000000009', 'SVC-CONSULT-01','IT Consulting (per day)',       'Services',        'Expert IT consulting services billed per day',                     1200.00,  350.00,   0,   0, 'Internal',             true),
  ('p1000000-0000-0000-0000-000000000010', 'SVC-SUPPORT-01','24/7 Support Contract (annual)','Services',       'Annual 24/7 priority support agreement',                           5999.00,  900.00,   0,   0, 'Internal',             true),
  ('p1000000-0000-0000-0000-000000000011', 'CLOUD-STOR-001','Cloud Storage 1TB/month',       'Cloud',           'Managed cloud storage 1TB monthly subscription',                     99.00,   15.00,   0,   0, 'CloudBase Inc',        true),
  ('p1000000-0000-0000-0000-000000000012', 'CLOUD-COMP-001','Cloud Compute (per vCPU-hr)',   'Cloud',           'On-demand cloud compute per vCPU-hour',                               0.08,   0.02,   0,   0, 'CloudBase Inc',        true),
  ('p1000000-0000-0000-0000-000000000013', 'SOFT-HR-001',   'HR Management Platform',       'Software',        'End-to-end HR lifecycle management solution',                      1799.00,  280.00, 400,  40, 'PeopleFirst Tech',     true),
  ('p1000000-0000-0000-0000-000000000014', 'HW-MONITOR-001','UltraWide 34" Monitor',        'Hardware',        '34-inch curved ultrawide monitor, 144Hz, 4K resolution',            899.00,  530.00,  95,  10, 'DisplayTech Corp',     true),
  ('p1000000-0000-0000-0000-000000000015', 'SOFT-AI-001',   'AI Analytics Accelerator',     'Software',        'AI-powered analytics and predictive modeling platform',             3499.00,  600.00, 200,  20, 'AISystems Ltd',        true)
ON CONFLICT DO NOTHING;

-- ─── Inventory ──────────────────────────────────────────────────────────────

INSERT INTO enterprise_inventory (product_id, warehouse, quantity, location_code) VALUES
  ('p1000000-0000-0000-0000-000000000001', 'West Coast DC',   250, 'WC-A1-01'),
  ('p1000000-0000-0000-0000-000000000001', 'East Coast DC',   250, 'EC-B2-03'),
  ('p1000000-0000-0000-0000-000000000002', 'West Coast DC',   150, 'WC-A1-02'),
  ('p1000000-0000-0000-0000-000000000002', 'Central DC',      100, 'CD-C3-07'),
  ('p1000000-0000-0000-0000-000000000003', 'West Coast DC',   400, 'WC-A2-01'),
  ('p1000000-0000-0000-0000-000000000003', 'East Coast DC',   350, 'EC-B1-05'),
  ('p1000000-0000-0000-0000-000000000004', 'West Coast DC',   600, 'WC-B1-01'),
  ('p1000000-0000-0000-0000-000000000004', 'East Coast DC',   600, 'EC-A1-02'),
  ('p1000000-0000-0000-0000-000000000005', 'West Coast DC',   180, 'WC-A3-01'),
  ('p1000000-0000-0000-0000-000000000006', 'West Coast DC',    90, 'WC-C1-01'),
  ('p1000000-0000-0000-0000-000000000006', 'East Coast DC',    90, 'EC-C1-03'),
  ('p1000000-0000-0000-0000-000000000007', 'West Coast DC',    25, 'WC-D1-01'),
  ('p1000000-0000-0000-0000-000000000007', 'East Coast DC',    20, 'EC-D1-01'),
  ('p1000000-0000-0000-0000-000000000008', 'West Coast DC',    70, 'WC-C2-01'),
  ('p1000000-0000-0000-0000-000000000008', 'East Coast DC',    50, 'EC-C2-02')
ON CONFLICT DO NOTHING;

-- ─── Sales ──────────────────────────────────────────────────────────────────

INSERT INTO enterprise_sales (transaction_id, product_id, employee_id, quantity, unit_price, total_amount, customer_name, customer_email, region, sale_date, status) VALUES
  ('TXN-2024-0001', 'p1000000-0000-0000-0000-000000000001', 'e1000000-0000-0000-0000-000000000008',  5, 2499.00, 12495.00, 'Acme Corp',            'buyer@acme.com',        'North East', '2024-01-05 09:30:00', 'completed'),
  ('TXN-2024-0002', 'p1000000-0000-0000-0000-000000000002', 'e1000000-0000-0000-0000-000000000009',  2, 4999.00,  9998.00, 'GlobalTech Inc',        'orders@globaltech.com', 'West',       '2024-01-08 14:00:00', 'completed'),
  ('TXN-2024-0003', 'p1000000-0000-0000-0000-000000000003', 'e1000000-0000-0000-0000-000000000008', 10, 1899.00, 18990.00, 'Midwest Industries',   'it@midwest.com',        'Midwest',    '2024-01-12 11:15:00', 'completed'),
  ('TXN-2024-0004', 'p1000000-0000-0000-0000-000000000006', 'e1000000-0000-0000-0000-000000000009', 20, 1499.00, 29980.00, 'StartupXYZ',           'ops@startupxyz.com',    'West',       '2024-01-15 10:00:00', 'completed'),
  ('TXN-2024-0005', 'p1000000-0000-0000-0000-000000000004', 'e1000000-0000-0000-0000-000000000008', 50,  599.00, 29950.00, 'RemoteTeam LLC',       'admin@remoteteam.io',   'South',      '2024-01-18 15:30:00', 'completed'),
  ('TXN-2024-0006', 'p1000000-0000-0000-0000-000000000010', 'e1000000-0000-0000-0000-000000000009',  3, 5999.00, 17997.00, 'FinancialPro Group',   'it@finpro.com',         'North East', '2024-01-22 13:00:00', 'completed'),
  ('TXN-2024-0007', 'p1000000-0000-0000-0000-000000000007', 'e1000000-0000-0000-0000-000000000008',  4, 8999.00, 35996.00, 'DataCenter Solutions', 'procurement@dcs.com',   'West',       '2024-01-25 09:00:00', 'completed'),
  ('TXN-2024-0008', 'p1000000-0000-0000-0000-000000000005', 'e1000000-0000-0000-0000-000000000009',  8, 3299.00, 26392.00, 'SecureCo Enterprise',  'sec@secureco.com',      'Midwest',    '2024-01-28 16:45:00', 'completed'),
  ('TXN-2024-0009', 'p1000000-0000-0000-0000-000000000015', 'e1000000-0000-0000-0000-000000000008',  6, 3499.00, 20994.00, 'Analytics Firm',       'cto@analyticsfirm.com', 'East',       '2024-02-02 10:30:00', 'completed'),
  ('TXN-2024-0010', 'p1000000-0000-0000-0000-000000000001', 'e1000000-0000-0000-0000-000000000009', 12, 2499.00, 29988.00, 'RetailChain Group',    'it@retailchain.com',    'South',      '2024-02-05 11:00:00', 'completed'),
  ('TXN-2024-0011', 'p1000000-0000-0000-0000-000000000013', 'e1000000-0000-0000-0000-000000000008', 15, 1799.00, 26985.00, 'HRConsulting Co',      'ops@hrco.com',          'North East', '2024-02-10 14:30:00', 'completed'),
  ('TXN-2024-0012', 'p1000000-0000-0000-0000-000000000003', 'e1000000-0000-0000-0000-000000000009', 20, 1899.00, 37980.00, 'BI Analytics Corp',    'data@biac.com',         'West',       '2024-02-14 09:15:00', 'completed'),
  ('TXN-2024-0013', 'p1000000-0000-0000-0000-000000000011', 'e1000000-0000-0000-0000-000000000008',100,   99.00,  9900.00, 'CloudFirst Inc',       'it@cloudfirst.io',      'West',       '2024-02-18 12:00:00', 'completed'),
  ('TXN-2024-0014', 'p1000000-0000-0000-0000-000000000002', 'e1000000-0000-0000-0000-000000000009',  3, 4999.00, 14997.00, 'ManufacturingPlus',    'erp@mfgplus.com',       'Midwest',    '2024-02-22 15:00:00', 'completed'),
  ('TXN-2024-0015', 'p1000000-0000-0000-0000-000000000006', 'e1000000-0000-0000-0000-000000000008', 30, 1499.00, 44970.00, 'TechStartup Hub',      'admin@tsh.io',          'West',       '2024-02-26 10:45:00', 'completed'),
  ('TXN-2024-0016', 'p1000000-0000-0000-0000-000000000004', 'e1000000-0000-0000-0000-000000000009', 75,  599.00, 44925.00, 'CollabFirst Agency',   'ops@collab.agency',     'East',       '2024-03-01 09:30:00', 'completed'),
  ('TXN-2024-0017', 'p1000000-0000-0000-0000-000000000005', 'e1000000-0000-0000-0000-000000000008',  5, 3299.00, 16495.00, 'HealthTech Systems',   'ciso@healthtech.com',   'South',      '2024-03-05 13:30:00', 'completed'),
  ('TXN-2024-0018', 'p1000000-0000-0000-0000-000000000001', 'e1000000-0000-0000-0000-000000000009', 18, 2499.00, 44982.00, 'EduPlatform Inc',      'it@eduplat.com',        'North East', '2024-03-10 11:00:00', 'completed'),
  ('TXN-2024-0019', 'p1000000-0000-0000-0000-000000000014', 'e1000000-0000-0000-0000-000000000008', 40,  899.00, 35960.00, 'DesignStudio Pro',     'gear@designstudio.com', 'West',       '2024-03-15 14:00:00', 'completed'),
  ('TXN-2024-0020', 'p1000000-0000-0000-0000-000000000015', 'e1000000-0000-0000-0000-000000000009', 10, 3499.00, 34990.00, 'InsurTech Partners',   'analytics@insur.com',   'Midwest',    '2024-03-20 10:00:00', 'completed'),
  ('TXN-2024-0021', 'p1000000-0000-0000-0000-000000000007', 'e1000000-0000-0000-0000-000000000008',  6, 8999.00, 53994.00, 'DataVault Corp',       'dc@datavault.com',      'East',       '2024-03-25 15:15:00', 'completed'),
  ('TXN-2024-0022', 'p1000000-0000-0000-0000-000000000010', 'e1000000-0000-0000-0000-000000000009',  5, 5999.00, 29995.00, 'TechGiant Ltd',        'support@tg.com',        'West',       '2024-04-01 09:00:00', 'completed'),
  ('TXN-2024-0023', 'p1000000-0000-0000-0000-000000000003', 'e1000000-0000-0000-0000-000000000008', 25, 1899.00, 47475.00, 'AnalyticsPro Group',   'bi@apg.com',            'South',      '2024-04-05 12:30:00', 'completed'),
  ('TXN-2024-0024', 'p1000000-0000-0000-0000-000000000001', 'e1000000-0000-0000-0000-000000000009',  8, 2499.00, 19992.00, 'ServiceNow Partner',   'crm@snpartner.com',     'North East', '2024-04-10 11:45:00', 'completed'),
  ('TXN-2024-0025', 'p1000000-0000-0000-0000-000000000013', 'e1000000-0000-0000-0000-000000000008', 22, 1799.00, 39578.00, 'HRDirect Corp',        'it@hrdirect.com',       'Midwest',    '2024-04-15 14:15:00', 'completed')
ON CONFLICT DO NOTHING;

-- ─── Additional demo users ──────────────────────────────────────────────────

INSERT INTO users (email, name, role, department, is_active) VALUES
  ('analyst@enterprise.com', 'Data Analyst',   'analyst', 'Finance',      true),
  ('user@enterprise.com',    'Regular User',   'user',    'Engineering',  true),
  ('manager@enterprise.com', 'Team Manager',   'admin',   'Operations',   true)
ON CONFLICT (email) DO NOTHING;
