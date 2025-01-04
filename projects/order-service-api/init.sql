-- Criação da tabela de inventário
CREATE TABLE IF NOT EXISTS tb_inventory (
                                            id SERIAL PRIMARY KEY,
                                            item_name VARCHAR(255) NOT NULL,
                                            quantity INT NOT NULL
);

-- Inserção de dados iniciais
INSERT INTO tb_inventory (item_name, quantity) VALUES
                                                   ('item1', 100),
                                                   ('item2', 200),
                                                   ('item3', 150),
                                                   ('item4', 300);
