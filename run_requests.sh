#!/bin/bash

# Configurações principais
URL="http://localhost:9090/order"               # URL do endpoint
DATA='{"item_name": "item1", "quantity": 1}'    # Dados da requisição
OUTPUT_FILE="responses.log"                     # Arquivo de saída
CONCURRENT_REQUESTS=20                          # Número de requisições simultâneas por lote
TOTAL_BATCHES=200                               # Número total de lotes
BATCH_SLEEP=0.1                                 # Intervalo entre lotes (em segundos)

# Exibe as configurações principais
show_config() {
  echo "==============================================="
  echo "           Script Configuration               "
  echo "==============================================="
  printf "%-25s: %s\n" "Endpoint URL" "$URL"
  printf "%-25s: %s\n" "Request Data" "$DATA"
  printf "%-25s: %s\n" "Output File" "$OUTPUT_FILE"
  printf "%-25s: %d\n" "Concurrent Requests" "$CONCURRENT_REQUESTS"
  printf "%-25s: %d\n" "Total Batches" "$TOTAL_BATCHES"
  printf "%-25s: %.1fs\n" "Batch Sleep Interval" "$BATCH_SLEEP"
  echo "==============================================="
}

# Verifica e remove o arquivo de saída, se existir
if [ -f "$OUTPUT_FILE" ]; then
  rm "$OUTPUT_FILE"
fi

# Função para realizar uma requisição
make_request() {
  RESPONSE=$(curl -s -o /dev/null -w "%{http_code}" -X POST -H "Content-Type: application/json" -d "$DATA" "$URL")
  echo "$RESPONSE"
}

# Inicializa os contadores
SUCCESS_COUNT=0
ERROR_COUNT=0

# Função para atualizar o progresso e sumário em uma única linha
update_progress_and_summary() {
  local current_batch=$1
  local total_batches=$2
  local percent=$((current_batch * 100 / total_batches))
  local progress=$((percent / 2))

  printf "\r[%-50s] %d%% (%d/%d) | Successful: %d | Failed: %d" \
    "$(printf '#%.0s' $(seq 1 $progress))" \
    "$percent" \
    "$current_batch" \
    "$total_batches" \
    "$SUCCESS_COUNT" \
    "$ERROR_COUNT"
}

# Exibe as configurações
show_config

# Loop principal para os lotes de requisições
for ((batch = 1; batch <= TOTAL_BATCHES; batch++)); do
  # Armazena os resultados de cada lote
  BATCH_RESULTS=()

  # Realiza CONCURRENT_REQUESTS em paralelo
  for ((i = 1; i <= CONCURRENT_REQUESTS; i++)); do
    BATCH_RESULTS+=("$(make_request &)")
  done

  # Aguarda todas as requisições deste lote serem concluídas
  wait

  # Atualiza os contadores com base nos resultados
  for response in "${BATCH_RESULTS[@]}"; do
    if [ "$response" -eq 200 ]; then
      ((SUCCESS_COUNT++))
    else
      ((ERROR_COUNT++))
    fi
    echo "HTTP Code: $response" >> "$OUTPUT_FILE"
  done

  # Atualiza a barra de progresso e o sumário em tempo real
  update_progress_and_summary $batch $TOTAL_BATCHES

  # Intervalo entre os lotes
  sleep $BATCH_SLEEP
done

# Finaliza a barra de progresso
update_progress_and_summary $TOTAL_BATCHES $TOTAL_BATCHES
echo -e "\nAll requests completed. Responses saved to $OUTPUT_FILE."
