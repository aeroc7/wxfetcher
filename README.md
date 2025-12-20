# WxFetcher ingests environment sensor data

WxFetcher ingests data (using rtl_433) from an outdoor weather station, in addition to multiple independent sensors ran by an esp32 (including a BMP390 and SCD30). Data is stored in InfluxDB and queried with a Grafana frontend:

<img width="1900" height="1045" alt="image" src="https://github.com/user-attachments/assets/e221d5db-817e-4f60-82b0-0c34d9786dbf" />

Also implemented are additional locations (an indoor room) for monitoring - still a WIP:
<img width="1900" height="746" alt="image" src="https://github.com/user-attachments/assets/4f9d8030-e127-40db-9886-f894e7d849ff" />

