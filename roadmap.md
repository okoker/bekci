
# BEKÇİ ROADMAP 

***2026***

I want to slowly move this monitoring tool from "its down" to "its going down" territory, from an end user point of view.  Ive seen so many monitoring solutions that show a device up when the help line is swamped with people complaining why _that server_ is soooo slow or not working at all. Changes in response time can be a good indicator of something being wrong.  So the next things I want to add are time based checks that can also be gated with the checks that currently exist.

  * HTTP/HTTPS response time checks.
  * TCP Three-Way Handshake Time (SYN to SYN-ACK) checks.
  * Time to First Byte (TTFB)
  * (TLS Client Hello to Server Hello Time)

* Delivery path monitoring - Monitoring all the bits and pieces my traffic goes through in an interrelated manner so I can have an idea on what might be effected given any one box is coughing.



If there is any interest I will consider adding;
* Telegram alarming
* Whatsapp alarming 
* SMS alarming
* AD Integration

 
---

Have a suggestion, need a feature, found a bug? Open an issue on the [Bekci repository](https://github.com/okoker/bekci).
