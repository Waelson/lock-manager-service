#!/bin/bash

# Configurações principais
URL="http://localhost:9090/order"               # URL do endpoint
DATA='{"item_name": "item1", "quantity": 1}'    # Dados da requisição
OUTPUT_FILE="responses.log"                     # Arquivo de saída
CONCURRENT_REQUESTS=10                          # Número de requisições simultâneas por lote
TOTAL_BATCHES=10                                # Número total de lotes
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
  echo "Removing existing file: $OUTPUT_FILE"
  rm "$OUTPUT_FILE"
fi

# Função para exibir a barra de progresso
show_progress() {
  local current=$1
  local total=$2
  local percent=$((current * 100 / total))
  local progress=$((percent / 2))
  printf "\r[%-50s] %d%% (%d/%d)" "$(printf '#%.0s' $(seq 1 $progress))" "$percent" "$current" "$total"
}

# Função para realizar uma requisição
make_request() {
  RESPONSE=$(curl -s -o /dev/null -w "%{http_code}" -X POST -H "Content-Type: application/json" -d "$DATA" "$URL")
  echo "$RESPONSE"
}

# Exibe as configurações
show_config

# Inicializa os contadores
SUCCESS_COUNT=0
ERROR_COUNT=0

# Loop principal para os lotes de requisições
for ((batch = 1; batch <= TOTAL_BATCHES; batch++)); do
  # Mostra o progresso
  show_progress $batch $TOTAL_BATCHES

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

  # Intervalo entre os lotes
  sleep $BATCH_SLEEP
done

# Finaliza a barra de progresso
show_progress $TOTAL_BATCHES $TOTAL_BATCHES
echo -e "\nAll requests completed. Responses saved to $OUTPUT_FILE."

# Exibe o resumo
echo "==============================================="
echo "                   Summary                    "
echo "==============================================="
printf "%-25s: %d\n" "Successful Requests" "$SUCCESS_COUNT"
printf "%-25s: %d\n" "Failed Requests" "$ERROR_COUNT"
echo "==============================================="
