#!/bin/bash

cd "$(dirname "$0")/.." || exit

GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m' 

echo -e "${BLUE}=====================================================${NC}"
echo -e "${BLUE}   🚀 Corporate Event Ticketing System (Load Test)   ${NC}"
echo -e "${BLUE}=====================================================${NC}"

# echo "DELETE FROM applications WHERE user_id IN (SELECT id FROM users WHERE employee_id LIKE 'STRESS%'); DELETE FROM ticket_types WHERE name = '壓測票'; DELETE FROM events WHERE title = '8萬人極限壓測'; DELETE FROM users WHERE employee_id LIKE 'STRESS%';" | docker compose exec -T postgres psql -q -U ts_user -d ticketing_system > /dev/null 2>&1

echo -e "\n${YELLOW}[1/4] 📦 Generating testing data ...${NC}"
node load-test/generate_data.js

echo -e "\n${YELLOW}[2/4] 💉 Inserting the data into PostgreSQL ...${NC}"
cat load-test/setup_db.sql | docker compose exec -T postgres psql -q -U ts_user -d ticketing_system > /dev/null

sleep 2

echo -e "\n${YELLOW}[3/4] 🔥 starting k6 test ...${NC}"
k6 run --summary-export=load-test/book-summary.json load-test/book-ticket.js

echo -e "\n${YELLOW}[4/4] 🧹 Deleting test data ...${NC}"
echo "
-- 1. tickets
DELETE FROM tickets WHERE application_id IN (SELECT id FROM applications WHERE user_id IN (SELECT id FROM users WHERE email LIKE 'stress%'));

-- 2. applications
DELETE FROM applications WHERE user_id IN (SELECT id FROM users WHERE email LIKE 'stress%');

-- 3. ticket_types
DELETE FROM ticket_types WHERE name = '壓測票';

-- 4. events
DELETE FROM events WHERE title = '8萬人極限壓測';

-- 5. manager
DELETE FROM users WHERE email LIKE 'stress%';
" | docker compose exec -T postgres psql -q -U ts_user -d ticketing_system

echo -e "\n${GREEN}✅ Finished!${NC}"
echo -e "${BLUE}=====================================================${NC}"
