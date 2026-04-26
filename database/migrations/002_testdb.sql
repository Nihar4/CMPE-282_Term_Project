-- =============================================================
-- Test DB Schema (datacharmer/test_db - PostgreSQL port)
-- Based on: https://github.com/datacharmer/test_db
-- =============================================================

-- ── Core Tables ───────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS departments (
    dept_no   CHAR(4)      NOT NULL,
    dept_name VARCHAR(40)  NOT NULL,
    PRIMARY KEY (dept_no),
    UNIQUE (dept_name)
);

CREATE TABLE IF NOT EXISTS employees (
    emp_no      INT         NOT NULL,
    birth_date  DATE        NOT NULL,
    first_name  VARCHAR(14) NOT NULL,
    last_name   VARCHAR(16) NOT NULL,
    gender      CHAR(1)     NOT NULL CHECK (gender IN ('M','F')),
    hire_date   DATE        NOT NULL,
    PRIMARY KEY (emp_no)
);

CREATE TABLE IF NOT EXISTS dept_manager (
    emp_no    INT    NOT NULL,
    dept_no   CHAR(4) NOT NULL,
    from_date DATE   NOT NULL,
    to_date   DATE   NOT NULL,
    FOREIGN KEY (emp_no)  REFERENCES employees(emp_no)   ON DELETE CASCADE,
    FOREIGN KEY (dept_no) REFERENCES departments(dept_no) ON DELETE CASCADE,
    PRIMARY KEY (emp_no, dept_no)
);

CREATE TABLE IF NOT EXISTS dept_emp (
    emp_no    INT    NOT NULL,
    dept_no   CHAR(4) NOT NULL,
    from_date DATE   NOT NULL,
    to_date   DATE   NOT NULL,
    FOREIGN KEY (emp_no)  REFERENCES employees(emp_no)   ON DELETE CASCADE,
    FOREIGN KEY (dept_no) REFERENCES departments(dept_no) ON DELETE CASCADE,
    PRIMARY KEY (emp_no, dept_no)
);

CREATE TABLE IF NOT EXISTS titles (
    emp_no    INT         NOT NULL,
    title     VARCHAR(50) NOT NULL,
    from_date DATE        NOT NULL,
    to_date   DATE,
    FOREIGN KEY (emp_no) REFERENCES employees(emp_no) ON DELETE CASCADE,
    PRIMARY KEY (emp_no, title, from_date)
);

CREATE TABLE IF NOT EXISTS salaries (
    emp_no    INT  NOT NULL,
    salary    INT  NOT NULL,
    from_date DATE NOT NULL,
    to_date   DATE NOT NULL,
    FOREIGN KEY (emp_no) REFERENCES employees(emp_no) ON DELETE CASCADE,
    PRIMARY KEY (emp_no, from_date)
);

-- ── Indexes for performance ────────────────────────────────────

CREATE INDEX IF NOT EXISTS idx_emp_name      ON employees(last_name, first_name);
CREATE INDEX IF NOT EXISTS idx_emp_hire      ON employees(hire_date);
CREATE INDEX IF NOT EXISTS idx_emp_gender    ON employees(gender);
CREATE INDEX IF NOT EXISTS idx_dept_emp_emp  ON dept_emp(emp_no);
CREATE INDEX IF NOT EXISTS idx_dept_emp_dept ON dept_emp(dept_no);
CREATE INDEX IF NOT EXISTS idx_titles_emp    ON titles(emp_no);
CREATE INDEX IF NOT EXISTS idx_salaries_emp  ON salaries(emp_no);
CREATE INDEX IF NOT EXISTS idx_salaries_amt  ON salaries(salary);

-- ── Useful Views ──────────────────────────────────────────────

CREATE OR REPLACE VIEW dept_emp_latest_date AS
    SELECT emp_no, MAX(from_date) AS from_date, MAX(to_date) AS to_date
    FROM dept_emp
    GROUP BY emp_no;

CREATE OR REPLACE VIEW current_dept_emp AS
    SELECT l.emp_no, dept_no, l.from_date, l.to_date
    FROM dept_emp d
    INNER JOIN dept_emp_latest_date l
        ON d.emp_no = l.emp_no
        AND d.from_date = l.from_date
        AND l.to_date = d.to_date;

-- ── Helper Functions (PL/pgSQL) ────────────────────────────────

CREATE OR REPLACE FUNCTION emp_dept_id(employee_id INT)
RETURNS CHAR(4) LANGUAGE plpgsql STABLE AS $$
DECLARE
    max_date DATE;
    result   CHAR(4);
BEGIN
    SELECT MAX(from_date) INTO max_date FROM dept_emp WHERE emp_no = employee_id;
    SELECT dept_no INTO result FROM dept_emp WHERE emp_no = employee_id AND from_date = max_date LIMIT 1;
    RETURN result;
END; $$;

CREATE OR REPLACE FUNCTION emp_dept_name(employee_id INT)
RETURNS VARCHAR(40) LANGUAGE plpgsql STABLE AS $$
BEGIN
    RETURN (SELECT dept_name FROM departments WHERE dept_no = emp_dept_id(employee_id));
END; $$;

CREATE OR REPLACE FUNCTION emp_name(employee_id INT)
RETURNS VARCHAR(32) LANGUAGE plpgsql STABLE AS $$
BEGIN
    RETURN (SELECT concat(first_name, ' ', last_name) FROM employees WHERE emp_no = employee_id);
END; $$;

CREATE OR REPLACE FUNCTION current_manager(dept_id CHAR(4))
RETURNS VARCHAR(32) LANGUAGE plpgsql STABLE AS $$
BEGIN
    RETURN (
        SELECT concat(e.first_name, ' ', e.last_name)
        FROM dept_manager dm
        JOIN employees e ON dm.emp_no = e.emp_no
        WHERE dm.dept_no = dept_id AND dm.to_date = '9999-01-01'
        LIMIT 1
    );
END; $$;
