
# BEKÇİ ROADMAP 

***2026***

I want to gradually move this monitoring tool from "it's down" territory to "it's going down" territory, from an end-user point of view. I've seen so many monitoring solutions report a device as up while the help line is swamped with people complaining about how _that server_ is painfully slow or not working at all. Changes in response time can be a good early indicator that something is wrong. So the next thing I want to add is time-based checks — which can also be gated by the checks that already exist.

  * HTTP/HTTPS response time checks.
  * TCP Three-Way Handshake Time (SYN to SYN-ACK) checks.
  * Time to First Byte (TTFB)
  * (TLS Client Hello to Server Hello Time)

* Delivery path monitoring - Monitoring all the bits and pieces my traffic goes through in an interrelated manner so I can have an idea of what might be affected when any one box is coughing.



If there is any interest I will consider adding;
* Telegram alarming
* Whatsapp alarming 
* SMS alarming
* AD Integration

 
---

Have a suggestion, need a feature, found a bug? Open an issue on the [Bekci repository](https://github.com/okoker/bekci).
