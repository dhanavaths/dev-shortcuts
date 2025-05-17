
import threading

def _wait_forever():
    e = threading.Event()
    e.wait()

def _main():
    # start webserver if required
    _wait_forever()

if __name__ == "__main__":
    _main()
